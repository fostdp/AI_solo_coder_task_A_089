package structural_simulator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"plankroad-backend/config"
	"plankroad-backend/models"
	"plankroad-backend/modules/bus"
	"plankroad-backend/repository"
	"plankroad-backend/simulation"
)

type StructuralSimulator struct {
	cfg           *config.Config
	bus           *bus.Bus
	logger        *log.Logger
	siteRepo      *repository.SiteRepo
	simRepo       *repository.SimulationRepo
	sensorRepo    *repository.SensorRepo
	mqttPub       MQTTPublisher
	siteNames     map[int]string
	workerPool    chan struct{}
	simulateMutex sync.Mutex
	metrics       struct {
		Simulations      uint64
		Success          uint64
		Failed           uint64
		AvgIterations    float64
		TotalComputeTime time.Duration
		TriggeredByTimer uint64
		TriggeredByAPI   uint64
		TriggeredByBus   uint64
	}
}

type MQTTPublisher interface {
	PublishSimulation(sim *models.StructuralSimulation) error
}

func New(cfg *config.Config, b *bus.Bus, logger *log.Logger, mqttPub MQTTPublisher) (*StructuralSimulator, error) {
	if cfg == nil || b == nil {
		return nil, fmt.Errorf("config and bus must not be nil")
	}

	siteRepo := repository.NewSiteRepo()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sites, err := siteRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load sites: %w", err)
	}

	siteNames := make(map[int]string, len(sites))
	for _, s := range sites {
		siteNames[s.SiteID] = s.SiteName
	}

	poolSize := cfg.FEM.Concurrency
	if poolSize <= 0 {
		poolSize = 3
	}

	s := &StructuralSimulator{
		cfg:        cfg,
		bus:        b,
		logger:     logger,
		siteRepo:   siteRepo,
		simRepo:    repository.NewSimulationRepo(),
		sensorRepo: repository.NewSensorRepo(),
		mqttPub:    mqttPub,
		siteNames:  siteNames,
		workerPool: make(chan struct{}, poolSize),
	}

	s.setupSubscriptions()
	return s, nil
}

func (s *StructuralSimulator) setupSubscriptions() {
	ctx := context.Background()

	s.bus.Subscribe(ctx, bus.EventCommandSimulate, "structsim-cmd", 10,
		func(ctx context.Context, evt bus.Event) {
			s.metrics.TriggeredByBus++
			s.logger.Printf("[SIM] Bus command triggered simulation for site %d", evt.SiteID)
			go s.RunForSite(evt.SiteID, "bus")
		})

	s.bus.Subscribe(ctx, bus.EventSensorDataValidated, "structsim-data", 100,
		func(ctx context.Context, evt bus.Event) {
			p, ok := evt.PayloadAsSensorData()
			if !ok {
				return
			}
			if p.IsBatch && len(p.Batch) >= 100 {
				s.logger.Printf("[SIM] Batch of %d received for site %d, scheduling simulation",
					len(p.Batch), evt.SiteID)
			}
		})
}

func (s *StructuralSimulator) RunForSite(siteID int, trigger string) (*models.StructuralSimulation, error) {
	s.simulateMutex.Lock()
	s.simulateMutex.Unlock()

	s.workerPool <- struct{}{}
	defer func() { <-s.workerPool }()

	start := time.Now()
	s.metrics.Simulations++

	switch trigger {
	case "timer":
		s.metrics.TriggeredByTimer++
	case "api":
		s.metrics.TriggeredByAPI++
	}

	ctx := context.Background()
	site, err := s.siteRepo.GetByID(ctx, siteID)
	if err != nil {
		s.metrics.Failed++
		return nil, fmt.Errorf("get site %d: %w", siteID, err)
	}

	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(s.cfg.FEM.RecentHours) * time.Hour)
	readings, err := s.sensorRepo.GetBySite(ctx, siteID, startTime, endTime, 1000)
	if err != nil {
		s.logger.Printf("[SIM] Warning: failed to get recent readings: %v", err)
	}

	avgTemp := 15.0
	if len(readings) > 0 {
		sum := 0.0
		for _, r := range readings {
			sum += r.Temperature
		}
		avgTemp = sum / float64(len(readings))
	}

	simResult, err := s.runFEM(site, avgTemp, readings)
	if err != nil {
		s.metrics.Failed++
		return nil, fmt.Errorf("FEM simulation failed: %w", err)
	}

	simResult.SiteID = siteID
	simResult.SimTime = time.Now()
	simResult.ComputeTimeMs = int64(time.Since(start).Milliseconds())

	ctxSave := context.Background()
	if err := s.simRepo.Save(ctxSave, simResult); err != nil {
		s.metrics.Failed++
		return nil, fmt.Errorf("save simulation: %w", err)
	}

	s.metrics.Success++
	s.metrics.TotalComputeTime += time.Since(start)
	s.metrics.AvgIterations = (s.metrics.AvgIterations*float64(s.metrics.Success-1) +
		float64(simResult.Iterations)) / float64(s.metrics.Success)

	payload := &bus.SimulationPayload{
		Simulation: simResult,
		Readings:   readings,
	}
	if err := s.bus.Publish(context.Background(), bus.Event{
		Type:      bus.EventStructuralSimReady,
		SiteID:    siteID,
		Timestamp: time.Now(),
		Payload:   payload,
	}); err != nil {
		s.logger.Printf("[SIM] Warning: publish simulation event failed: %v", err)
	}

	if s.mqttPub != nil {
		if err := s.mqttPub.PublishSimulation(simResult); err != nil {
			s.logger.Printf("[SIM] Warning: MQTT publish failed: %v", err)
		}
	}

	s.logger.Printf("[SIM] Site %d (%s) completed in %v, iterations=%d, SF=%.2f",
		siteID, site.SiteName, time.Since(start), simResult.Iterations, simResult.SafetyFactor)

	return simResult, nil
}

func (s *StructuralSimulator) RunAll(trigger string) error {
	ctx := context.Background()
	sites, err := s.siteRepo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("get sites: %w", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 0, len(sites))
	errMu := sync.Mutex{}

	for _, site := range sites {
		wg.Add(1)
		go func(siteID int) {
			defer wg.Done()
			if _, err := s.RunForSite(siteID, trigger); err != nil {
				errMu.Lock()
				errs = append(errs, fmt.Errorf("site %d: %w", siteID, err))
				errMu.Unlock()
			}
		}(site.SiteID)

		time.Sleep(100 * time.Millisecond)
	}

	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("some simulations failed: %v", errs)
	}
	return nil
}

func (s *StructuralSimulator) runFEM(site *models.PlankroadSite, avgTemp float64,
	readings []models.SensorReading) (*models.StructuralSimulation, error) {

	solver := simulation.NewSolver(&s.cfg.FEM)
	simResult, err := solver.Simulate(site, readings)
	if err != nil {
		return nil, fmt.Errorf("solve: %w", err)
	}

	return simResult, nil
}

func (s *StructuralSimulator) GetSiteNames() map[int]string {
	return s.siteNames
}

func (s *StructuralSimulator) GetSiteRepo() *repository.SiteRepo {
	return s.siteRepo
}

func (s *StructuralSimulator) GetSimRepo() *repository.SimulationRepo {
	return s.simRepo
}

func (s *StructuralSimulator) GetMetrics() map[string]interface{} {
	avgMs := int64(0)
	if s.metrics.Success > 0 {
		avgMs = int64(s.metrics.TotalComputeTime / time.Duration(s.metrics.Success) / time.Millisecond)
	}
	return map[string]interface{}{
		"simulations_total":   s.metrics.Simulations,
		"success":             s.metrics.Success,
		"failed":              s.metrics.Failed,
		"avg_iterations":      s.metrics.AvgIterations,
		"avg_compute_ms":      avgMs,
		"triggered_by_timer":  s.metrics.TriggeredByTimer,
		"triggered_by_api":    s.metrics.TriggeredByAPI,
		"triggered_by_bus":    s.metrics.TriggeredByBus,
	}
}
