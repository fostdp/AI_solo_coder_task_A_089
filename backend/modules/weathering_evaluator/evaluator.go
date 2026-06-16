package weathering_evaluator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"plankroad-backend/config"
	"plankroad-backend/config/params"
	"plankroad-backend/models"
	"plankroad-backend/modules/bus"
	"plankroad-backend/repository"
)

type FreezeThawState int

const (
	StateNormal FreezeThawState = iota
	StateFreezing
	StateFrozen
	StateThawing
)

type FreezeThawTracker struct {
	CurrentState    FreezeThawState
	Cycles          int
	LastStateChange time.Time
	FreezeDuration  time.Duration
	ThawDuration    time.Duration
	TotalFreezeHrs  float64
	TotalThawHrs    float64
	MaxFreezeRate   float64
	MaxThawRate     float64
}

type freezeStats struct {
	avgFreezeHrs    float64
	avgThawHrs      float64
	maxFreezeRate   float64
	maxThawRate     float64
	depthBelowZero  float64
}

type WeatheringEvaluator struct {
	cfg         *config.Config
	bus         *bus.Bus
	logger      *log.Logger
	siteRepo    *repository.SiteRepo
	sensorRepo  *repository.SensorRepo
	simRepo     *repository.SimulationRepo
	weatherRepo *repository.WeatheringRepo
	mqttPub     MQTTPublisher
	siteNames   map[int]string
	ftTrackers  map[int]*FreezeThawTracker
	ftTrackerMu sync.Mutex
	workerPool  chan struct{}
	metrics     struct {
		Assessments       uint64
		Success           uint64
		Failed            uint64
		AvgConfidence     float64
		TriggeredByTimer  uint64
		TriggeredByAPI    uint64
		TriggeredByBus    uint64
		TotalCycles       uint64
	}
}

type MQTTPublisher interface {
	PublishWeathering(wa *models.WeatheringAssessment) error
}

func New(cfg *config.Config, b *bus.Bus, logger *log.Logger, mqttPub MQTTPublisher) (*WeatheringEvaluator, error) {
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

	poolSize := cfg.Weather.Concurrency
	if poolSize <= 0 {
		poolSize = 2
	}

	e := &WeatheringEvaluator{
		cfg:         cfg,
		bus:         b,
		logger:      logger,
		siteRepo:    siteRepo,
		sensorRepo:  repository.NewSensorRepo(),
		simRepo:     repository.NewSimulationRepo(),
		weatherRepo: repository.NewWeatheringRepo(),
		mqttPub:     mqttPub,
		siteNames:   siteNames,
		ftTrackers:  make(map[int]*FreezeThawTracker),
		workerPool:  make(chan struct{}, poolSize),
	}

	e.setupSubscriptions()
	return e, nil
}

func (e *WeatheringEvaluator) setupSubscriptions() {
	ctx := context.Background()

	e.bus.Subscribe(ctx, bus.EventCommandWeathering, "weathering-cmd", 10,
		func(ctx context.Context, evt bus.Event) {
			e.metrics.TriggeredByBus++
			e.logger.Printf("[WEATHER] Bus command triggered assessment for site %d", evt.SiteID)
			go e.RunForSite(evt.SiteID, "bus")
		})

	e.bus.Subscribe(ctx, bus.EventStructuralSimReady, "weathering-sim", 20,
		func(ctx context.Context, evt bus.Event) {
			_, ok := evt.PayloadAsSimulation()
			if !ok {
				return
			}
			e.logger.Printf("[WEATHER] Simulation ready for site %d, scheduling assessment", evt.SiteID)
		})
}

func (e *WeatheringEvaluator) RunForSite(siteID int, trigger string) (*models.WeatheringAssessment, error) {
	e.workerPool <- struct{}{}
	defer func() { <-e.workerPool }()

	start := time.Now()
	e.metrics.Assessments++

	switch trigger {
	case "timer":
		e.metrics.TriggeredByTimer++
	case "api":
		e.metrics.TriggeredByAPI++
	}

	ctx := context.Background()
	site, err := e.siteRepo.GetByID(ctx, siteID)
	if err != nil {
		e.metrics.Failed++
		return nil, fmt.Errorf("get site %d: %w", siteID, err)
	}

	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(e.cfg.Weather.RecentHours) * time.Hour)
	readings, err := e.sensorRepo.GetBySite(ctx, siteID, startTime, endTime, 1000)
	if err != nil {
		e.logger.Printf("[WEATHER] Warning: failed to get recent readings: %v", err)
		readings = []models.SensorReading{}
	}

	sim, err := e.simRepo.GetLatest(ctx, siteID)
	if err != nil {
		e.logger.Printf("[WEATHER] Warning: no simulation found: %v", err)
	}

	wa, err := e.Assess(site, readings, sim)
	if err != nil {
		e.metrics.Failed++
		return nil, fmt.Errorf("assess: %w", err)
	}

	if err := e.weatherRepo.Save(ctx, wa); err != nil {
		e.metrics.Failed++
		return nil, fmt.Errorf("save assessment: %w", err)
	}

	e.metrics.Success++
	e.metrics.AvgConfidence = (e.metrics.AvgConfidence*float64(e.metrics.Success-1) +
		wa.Confidence) / float64(e.metrics.Success)
	e.metrics.TotalCycles += uint64(wa.FreezeThawCycles)

	payload := &bus.WeatheringPayload{
		Assessment: wa,
		Readings:   readings,
		Sim:        sim,
	}
	if err := e.bus.Publish(context.Background(), bus.Event{
		Type:      bus.EventWeatheringReady,
		SiteID:    siteID,
		Timestamp: time.Now(),
		Payload:   payload,
	}); err != nil {
		e.logger.Printf("[WEATHER] Warning: publish event failed: %v", err)
	}

	if e.mqttPub != nil {
		if err := e.mqttPub.PublishWeathering(wa); err != nil {
			e.logger.Printf("[WEATHER] Warning: MQTT publish failed: %v", err)
		}
	}

	elapsed := time.Since(start)
	e.logger.Printf("[WEATHER] Site %d (%s) completed in %v, grade=%s, life=%.1fy",
		siteID, site.SiteName, elapsed, wa.WeatheringGrade, wa.PredictedLifespan)

	return wa, nil
}

func (e *WeatheringEvaluator) RunAll(trigger string) error {
	ctx := context.Background()
	sites, err := e.siteRepo.GetAll(ctx)
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
			if _, err := e.RunForSite(siteID, trigger); err != nil {
				errMu.Lock()
				errs = append(errs, fmt.Errorf("site %d: %w", siteID, err))
				errMu.Unlock()
			}
		}(site.SiteID)

		time.Sleep(100 * time.Millisecond)
	}

	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("some assessments failed: %v", errs)
	}
	return nil
}

func (e *WeatheringEvaluator) Assess(site *models.PlankroadSite, readings []models.SensorReading,
	sim *models.StructuralSimulation) (*models.WeatheringAssessment, error) {

	e.ftTrackerMu.Lock()
	if _, ok := e.ftTrackers[site.SiteID]; !ok {
		e.ftTrackers[site.SiteID] = &FreezeThawTracker{}
	}
	e.ftTrackerMu.Unlock()

	rp, _ := params.GetRockParams(site.RockType)
	wp, _ := params.GetWoodParams(site.WoodType)

	decayRp := convertRockParams(rp)
	decayWp := convertWoodParams(wp)

	cycles, freezeStats := e.calcFreezeThawCycles(site.SiteID, readings, decayRp)
	avgTemp, avgHumid, tempRange, minTemp, maxTemp, tempTrans := calcClimateParams(readings)

	avgCrack, maxCrack := calcCrackStats(readings)

	stressRange := 5.0
	if sim != nil {
		stressRange = math.Max(sim.MaxRockStress-sim.MinRockStress, 5.0)
	}

	ftDamage := calcFTDamage(decayRp, cycles, freezeStats)
	shockDamage := calcThermalShock(decayRp, tempRange, float64(tempTrans))
	chemDamage := calcChemicalErosion(decayRp, avgTemp, avgHumid)

	crackPropRate := e.calcCrackPropagation(decayRp, stressRange, avgCrack, cycles,
		ftDamage, shockDamage, chemDamage)
	crackRate := crackPropRate * 12.0

	beddingAccel := decayRp.BeddingFactor * (1.0 + decayRp.Thermal.AnisotropyIdx*0.1)
	crackPropRate *= math.Pow(beddingAccel, 0.3)
	crackRate = crackPropRate * 12.0

	weatherGrade := e.gradeWeathering(decayRp, maxCrack, crackRate, cycles, ftDamage, shockDamage)

	woodDecay := e.calcWoodDecayRate(decayWp, avgTemp, avgHumid, cycles, freezeStats)
	rockErosion := e.calcRockErosionRate(decayRp, avgHumid, tempRange, cycles, ftDamage, chemDamage)

	currentAge := e.estimateCurrentAge(site)
	ageDamageFactor := 1.0 + currentAge*0.0005*decayRp.ChemicalFactor

	rockLife := e.predictRockLifespan(maxCrack, crackPropRate, decayRp, ftDamage)
	woodLife := e.predictWoodLifespan(woodDecay, currentAge)
	predictedLife := math.Min(rockLife, woodLife)
	predictedLife /= ageDamageFactor

	remaining := math.Max(predictedLife-currentAge, 0.1)
	confidence := e.calcConfidence(len(readings), cycles, stressRange, decayRp)

	freezeDepression := calcFreezingPointDepression(decayRp)

	detail := map[string]interface{}{
		"avg_temperature":          round4(avgTemp),
		"min_temperature":          round4(minTemp),
		"max_temperature":          round4(maxTemp),
		"avg_humidity":             round4(avgHumid),
		"temperature_range":        round4(tempRange),
		"temp_transitions":         tempTrans,
		"avg_crack_width_mm":       round4(avgCrack),
		"max_crack_width_mm":       round4(maxCrack),
		"monthly_crack_rate_mm":    round4(crackRate),
		"rock_fracture_toughness":  decayRp.K_IC,
		"rock_porosity":            decayRp.Pore.Porosity,
		"rock_avg_pore_radius_nm":  decayRp.Pore.AvgPoreRadius * 1e9,
		"rock_permeability":        decayRp.Pore.Permeability,
		"freezing_point_depression": round4(freezeDepression),
		"effective_freeze_temp":    round4(e.cfg.Weather.FreezeTemp + freezeDepression),
		"freeze_pressure_mpa":      decayRp.FreezePressure,
		"ft_damage_index":          round4(ftDamage),
		"thermal_shock_index":      round4(shockDamage),
		"chemical_erosion_index":   round4(chemDamage),
		"thermal_anisotropy":       decayRp.Thermal.AnisotropyIdx,
		"bedding_acceleration":     round4(beddingAccel),
		"wood_lignin_content":      decayWp.LigninContent,
		"wood_moisture_content":    round4(decayWp.InitMoisture + avgHumid*0.1),
		"wood_durability_class":    decayWp.DurabilityClass,
		"estimated_age_years":      round4(currentAge),
		"age_damage_factor":        round4(ageDamageFactor),
		"rock_lifespan_years":      round4(rockLife),
		"wood_lifespan_years":      round4(woodLife),
		"rock_hardness_mohs":       decayRp.HardnessMohs,
		"rock_class":               decayRp.RockClass,
		"freeze_duration_hours":    round4(freezeStats.avgFreezeHrs),
		"thaw_duration_hours":      round4(freezeStats.avgThawHrs),
	}
	detailJSON, _ := json.Marshal(detail)

	return &models.WeatheringAssessment{
		SiteID:              site.SiteID,
		AssessTime:          time.Now(),
		FreezeThawCycles:    cycles,
		CurrentCrackDepth:   round4(maxCrack),
		CrackPropagationRate: round4(crackPropRate),
		WeatheringGrade:     weatherGrade,
		WoodDecayRate:       round4(woodDecay),
		RockErosionRate:     round4(rockErosion),
		PredictedLifespan:   round4(predictedLife),
		RemainingLifespan:   round4(remaining),
		Confidence:          round4(confidence),
		DetailData:          detailJSON,
	}, nil
}

func (e *WeatheringEvaluator) calcFreezeThawCycles(siteID int, readings []models.SensorReading,
	rp rockParams) (int, freezeStats) {

	e.ftTrackerMu.Lock()
	tracker := e.ftTrackers[siteID]
	e.ftTrackerMu.Unlock()

	stats := freezeStats{}

	if len(readings) < 2 {
		return tracker.Cycles + 80, stats
	}

	baseCycles := tracker.Cycles
	baseFreezeTemp := e.cfg.Weather.FreezeTemp
	baseThawTemp := e.cfg.Weather.ThawTemp

	fpDepression := calcFreezingPointDepression(rp)
	freezeT := baseFreezeTemp + fpDepression
	thawT := baseThawTemp + fpDepression*0.5

	prevTemp := readings[len(readings)-1].Temperature
	prevTime := readings[len(readings)-1].Time
	transitions := 0
	freezingPeriods := []float64{}
	thawingPeriods := []float64{}
	maxFreezeRate := 0.0
	maxThawRate := 0.0
	totalFreezeHrs := 0.0
	currentState := StateNormal
	cycleStart := readings[len(readings)-1].Time

	for i := len(readings) - 2; i >= 0; i-- {
		currTemp := readings[i].Temperature
		currTime := readings[i].Time
		hours := prevTime.Sub(currTime).Hours()
		if hours <= 0 {
			hours = 1.0
		}

		coolingRate := (prevTemp - currTemp) / hours
		if coolingRate > maxFreezeRate {
			maxFreezeRate = coolingRate
		}
		warmingRate := (currTemp - prevTemp) / hours
		if warmingRate > maxThawRate {
			maxThawRate = warmingRate
		}

		if prevTemp > thawT && currTemp < freezeT {
			transitions++
			if currentState != StateFreezing {
				currentState = StateFreezing
				cycleStart = currTime
			}
		} else if prevTemp < freezeT && currTemp > thawT {
			transitions++
			if currentState == StateFreezing {
				dur := cycleStart.Sub(currTime).Hours()
				if dur > 0 {
					freezingPeriods = append(freezingPeriods, dur)
				}
			}
			currentState = StateThawing
			thawingPeriods = append(thawingPeriods, hours)
		}

		if currTemp < freezeT {
			totalFreezeHrs += hours
		}

		prevTemp = currTemp
		prevTime = currTime
	}

	newCycles := transitions / 2
	tracker.Cycles = baseCycles + newCycles
	tracker.TotalFreezeHrs += totalFreezeHrs
	tracker.MaxFreezeRate = math.Max(tracker.MaxFreezeRate, maxFreezeRate)
	tracker.MaxThawRate = math.Max(tracker.MaxThawRate, maxThawRate)

	stats.depthBelowZero = math.Max(0, -math.Min(0, baseFreezeTemp-fpDepression))
	if len(freezingPeriods) > 0 {
		for _, d := range freezingPeriods {
			stats.avgFreezeHrs += d
		}
		stats.avgFreezeHrs /= float64(len(freezingPeriods))
	}
	if len(thawingPeriods) > 0 {
		for _, d := range thawingPeriods {
			stats.avgThawHrs += d
		}
		stats.avgThawHrs /= float64(len(thawingPeriods))
	}
	stats.maxFreezeRate = tracker.MaxFreezeRate
	stats.maxThawRate = tracker.MaxThawRate
	if stats.avgFreezeHrs == 0 {
		stats.avgFreezeHrs = 6.0
	}
	if stats.avgThawHrs == 0 {
		stats.avgThawHrs = 8.0
	}

	return baseCycles + newCycles + 80, stats
}

func (e *WeatheringEvaluator) calcCrackPropagation(rp rockParams, stressRange, crackWidth float64,
	cycles int, ftDamage, shockDamage, chemDamage float64) float64 {
	pi := math.Pi
	geoFactor := 1.12 * math.Sqrt(rp.BeddingFactor)

	if crackWidth <= 0.01 {
		crackWidth = 0.1
	}

	deltaK := geoFactor * stressRange * math.Sqrt(pi*crackWidth/1000.0)

	if deltaK >= rp.K_IC {
		return e.cfg.Weather.CriticalCrackDepth / 50.0
	}

	waterIceFactor := 1.0
	if rp.Pore.Porosity > 0.05 {
		waterIceFactor = 1.0 + 9.0*(rp.Pore.Porosity-0.05)
	}

	environFactor := (1.0 + ftDamage*3.0) * (1.0 + shockDamage*2.0) * (1.0 + chemDamage*1.5) * waterIceFactor

	da_dN := rp.C * math.Pow(deltaK, rp.M) * environFactor

	annualCycles := math.Max(float64(cycles), 80.0)
	annualGrowth := da_dN * 1e3 * annualCycles

	minRate := 0.001 + rp.Pore.Porosity*0.08 + ftDamage*0.01
	return math.Max(annualGrowth, minRate)
}

func (e *WeatheringEvaluator) calcWoodDecayRate(wp woodParams, avgTemp, avgHumid float64,
	cycles int, fs freezeStats) float64 {
	tempFactor := math.Exp(wp.TempCoeff * (avgTemp - 20.0))
	humidFactor := 1.0 + wp.HumidCoeff*(avgHumid-65.0)
	if humidFactor < 0.1 {
		humidFactor = 0.1
	}

	ftFactor := 1.0 + float64(cycles)*0.001*(1.0+(1.0-float64(wp.DurabilityClass)*0.2))

	moisture := wp.InitMoisture + avgHumid*0.08 + fs.avgFreezeHrs*0.05

	var moistureFactor float64
	if moisture < 20 {
		moistureFactor = 0.1 + (moisture / 20.0) * 0.5
	} else if moisture <= 40 {
		moistureFactor = 0.6 + ((moisture - 20.0) / 20.0) * 0.4
	} else {
		moistureFactor = 1.0 + (moisture-40.0)*0.02
	}

	ligninFactor := 1.0 + (0.35-wp.LigninContent)*3.0
	durabilityFactor := math.Pow(1.5, float64(3-wp.DurabilityClass))

	annualDecay := wp.DecayCoeff * tempFactor * humidFactor * moistureFactor *
		ftFactor * wp.BiologicalFactor * ligninFactor * durabilityFactor * 100.0

	return math.Max(annualDecay, 0.01)
}

func (e *WeatheringEvaluator) calcRockErosionRate(rp rockParams, avgHumid, tempRange float64,
	cycles int, ftDamage, chemDamage float64) float64 {
	physFactor := 0.003 * (tempRange / 30.0) * (1.0 + ftDamage*2.0)
	chemFactor := 0.005 * (avgHumid / 100.0) * (1.0 + chemDamage*3.0)
	ftFactor := float64(cycles) * 0.0002 * rp.Pore.Porosity * rp.FreezePressure * (1.0 + ftDamage)
	shockFactor := 0.001 * rp.ShockFactor * tempRange / 20.0

	hardnessFactor := 1.0 / math.Max(rp.HardnessMohs/4.0, 0.6)
	beddingFactor := math.Sqrt(rp.BeddingFactor)

	totalErosion := (physFactor + chemFactor + ftFactor + shockFactor) * 1000.0 * hardnessFactor * beddingFactor
	return math.Max(totalErosion, 0.005)
}

func (e *WeatheringEvaluator) gradeWeathering(rp rockParams, crackWidth, crackRate float64,
	cycles int, ftD, shockD float64) string {
	baseRate := 0.05 / math.Sqrt(rp.HardnessMohs)
	score := 0.0

	criticalNorm := e.cfg.Weather.CriticalCrackDepth / 10.0
	if crackWidth >= criticalNorm {
		score += 30
	} else if crackWidth >= criticalNorm*0.6 {
		score += 20
	} else if crackWidth >= criticalNorm*0.2 {
		score += 10
	}

	if crackRate >= baseRate*6 {
		score += 40
	} else if crackRate >= baseRate*2 {
		score += 25
	} else if crackRate >= baseRate {
		score += 10
	}

	if cycles >= 500 {
		score += 15
	} else if cycles >= 200 {
		score += 10
	} else if cycles >= 100 {
		score += 5
	}

	score += ftD * 25
	score += shockD * 15

	switch {
	case score >= 80:
		return "SEVERE"
	case score >= 55:
		return "SERIOUS"
	case score >= 30:
		return "MODERATE"
	case score >= 10:
		return "MILD"
	default:
		return "SLIGHT"
	}
}

func (e *WeatheringEvaluator) estimateCurrentAge(site *models.PlankroadSite) float64 {
	eraAges := map[string]float64{
		"战国":    2400,
		"战国-唐宋": 1500,
		"战国-明清": 1200,
		"秦汉":    2200,
		"秦汉-明清": 1000,
		"三国":    1800,
		"唐代":    1400,
		"唐宋":    1000,
		"明清":    400,
	}
	if age, ok := eraAges[site.ConstructionEra]; ok {
		return age
	}
	return 1000.0
}

func (e *WeatheringEvaluator) predictRockLifespan(crackWidth, growthRate float64, rp rockParams, ftDamage float64) float64 {
	if growthRate <= 0 {
		return e.cfg.Weather.RockDesignLife
	}

	criticalDepth := e.cfg.Weather.CriticalCrackDepth / math.Sqrt(rp.BeddingFactor)
	remainingDepth := criticalDepth - crackWidth
	if remainingDepth <= 0 {
		return 1.0
	}

	acceleratedRate := growthRate * (1.0 + ftDamage*1.5)
	yearsToCritical := remainingDepth / acceleratedRate

	factor := 1.0
	if crackWidth > 10 {
		factor *= 0.7
	}
	if crackWidth > 25 {
		factor *= 0.5
	}

	return math.Min(yearsToCritical*factor, e.cfg.Weather.RockDesignLife)
}

func (e *WeatheringEvaluator) predictWoodLifespan(decayRate, currentAge float64) float64 {
	if decayRate <= 0 {
		return e.cfg.Weather.WoodDesignLife
	}

	totalLife := (100.0 / decayRate) * 0.8

	if currentAge >= totalLife {
		return currentAge + 5.0
	}

	return totalLife
}

func (e *WeatheringEvaluator) calcConfidence(dataPoints int, cycles int, stressRange float64, rp rockParams) float64 {
	dataScore := math.Min(float64(dataPoints)/500.0, 1.0) * 0.3
	cycleScore := math.Min(float64(cycles)/300.0, 1.0) * 0.2
	stressScore := math.Min(stressRange/20.0, 1.0) * 0.15
	rockModelScore := 0.15 + 0.1*(1.0-1.0/rp.Thermal.AnisotropyIdx)
	physicsScore := 0.15

	total := dataScore + cycleScore + stressScore + rockModelScore + physicsScore
	return math.Min(math.Max(total, 0.3), 0.98)
}

func (e *WeatheringEvaluator) GetWeatherRepo() *repository.WeatheringRepo {
	return e.weatherRepo
}

func (e *WeatheringEvaluator) GetSimRepo() *repository.SimulationRepo {
	return e.simRepo
}

func (e *WeatheringEvaluator) GetSensorRepo() *repository.SensorRepo {
	return e.sensorRepo
}

func (e *WeatheringEvaluator) GetSiteRepo() *repository.SiteRepo {
	return e.siteRepo
}

func (e *WeatheringEvaluator) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"assessments_total":  e.metrics.Assessments,
		"success":            e.metrics.Success,
		"failed":             e.metrics.Failed,
		"avg_confidence":     round4(e.metrics.AvgConfidence),
		"total_cycles":       e.metrics.TotalCycles,
		"triggered_by_timer": e.metrics.TriggeredByTimer,
		"triggered_by_api":   e.metrics.TriggeredByAPI,
		"triggered_by_bus":   e.metrics.TriggeredByBus,
	}
}

type rockParams struct {
	K_IC       float64
	C          float64
	M          float64
	Pore       params.PoreStructure
	Thermal    params.ThermalProps
	FreezePressure float64
	FTDamageBeta   float64
	ChemicalFactor float64
	ShockFactor    float64
	BeddingFactor  float64
	RockClass      string
	HardnessMohs   float64
}

type woodParams struct {
	InitMoisture     float64
	DecayCoeff       float64
	TempCoeff        float64
	HumidCoeff       float64
	BiologicalFactor float64
	LigninContent    float64
	DurabilityClass  int
}

func convertRockParams(rp params.RockParams) rockParams {
	return rockParams{
		K_IC:       rp.K_IC,
		C:          rp.C,
		M:          rp.M,
		Pore: params.PoreStructure{
			Porosity:      rp.Pore.Porosity,
			AvgPoreRadius: rp.Pore.AvgPoreRadius * 1e-9,
			PoreSizeDisp:  rp.Pore.PoreSizeDisp,
			Saturation:    rp.Pore.Saturation,
			Permeability:  rp.Pore.Permeability,
		},
		Thermal: params.ThermalProps{
			AlphaParallel: rp.Thermal.AlphaParallel,
			AlphaNormal:   rp.Thermal.AlphaNormal,
			Conductivity:  rp.Thermal.Conductivity,
			HeatCapacity:  rp.Thermal.HeatCapacity,
			AnisotropyIdx: rp.Thermal.AnisotropyIdx,
		},
		FreezePressure: rp.FreezePressure,
		FTDamageBeta:   rp.FTDamageBeta,
		ChemicalFactor: rp.ChemicalFactor,
		ShockFactor:    rp.ShockFactor,
		BeddingFactor:  rp.BeddingFactor,
		RockClass:      rp.RockClass,
		HardnessMohs:   rp.HardnessMohs,
	}
}

func convertWoodParams(wp params.WoodParams) woodParams {
	return woodParams{
		InitMoisture:     wp.InitMoisture,
		DecayCoeff:       wp.DecayCoeff,
		TempCoeff:        wp.TempCoeff,
		HumidCoeff:       wp.HumidCoeff,
		BiologicalFactor: wp.BiologicalFactor,
		LigninContent:    wp.LigninContent,
		DurabilityClass:  wp.DurabilityClass,
	}
}

func calcFreezingPointDepression(rp rockParams) float64 {
	gammaSL := params.RockCommon.GammaSL
	latentHeat := params.RockCommon.LatentHeat
	rhoIce := params.RockCommon.RhoIce
	T0 := params.RockCommon.T0

	rpore := rp.Pore.AvgPoreRadius * rp.Pore.PoreSizeDisp * 0.5
	if rpore < 1e-10 {
		rpore = 1e-9
	}

	deltaT := (2.0 * gammaSL * T0) / (latentHeat * rhoIce * rpore)
	deltaT *= math.Pow(rp.Pore.Saturation, 0.67)

	return math.Min(deltaT, 5.0)
}

func calcFTDamage(rp rockParams, cycles int, fs freezeStats) float64 {
	D := 1.0 - math.Exp(-rp.FTDamageBeta*float64(cycles))

	pressureFactor := 1.0 + (rp.FreezePressure-2.0)*0.3
	porosityFactor := 1.0 + (rp.Pore.Porosity-0.03)*10.0
	saturationFactor := math.Pow(rp.Pore.Saturation, 1.5)
	rateFactor := 1.0 + (fs.maxFreezeRate+fs.maxThawRate)*0.01
	depthFactor := 1.0 + fs.depthBelowZero*0.15

	totalFactor := pressureFactor * porosityFactor * saturationFactor * rateFactor * depthFactor
	return math.Min(D*totalFactor, 0.99)
}

func calcThermalShock(rp rockParams, tempRange, tempTrans float64) float64 {
	shockBase := 1.0 - math.Exp(-rp.ShockFactor*0.0004*tempTrans)
	deltaT := math.Max(tempRange, 0.0)
	deltaTc := 20.0 * (2.5 / rp.HardnessMohs)
	shockThermal := math.Pow(deltaT/deltaTc, 1.5)
	anisotropy := math.Pow(rp.Thermal.AnisotropyIdx, 0.8)

	total := (shockBase*0.3 + shockThermal*0.7) * anisotropy
	return math.Min(total, 0.9)
}

func calcChemicalErosion(rp rockParams, avgTemp, avgHumid float64) float64 {
	tempFactor := math.Exp(0.08 * (avgTemp - 15.0))
	humidFactor := math.Pow(avgHumid/100.0, 1.3)

	carbonateFactor := 1.0
	if rp.RockClass == "carbonate" || rp.RockClass == "carbonate-metamorphic" {
		carbonateFactor = rp.ChemicalFactor * (1.0 + (avgHumid-70.0)*0.01)
	} else {
		carbonateFactor = rp.ChemicalFactor
	}

	permFactor := 1.0 + math.Log10(math.Max(rp.Pore.Permeability, 1e-20))*(-0.05)

	total := 0.01 * tempFactor * humidFactor * carbonateFactor * permFactor
	return math.Min(total, 0.5)
}

func calcClimateParams(readings []models.SensorReading) (avgTemp, avgHumid, tempRange, minTemp, maxTemp float64, transitions int) {
	if len(readings) == 0 {
		return 15.0, 70.0, 25.0, 5.0, 25.0, 50
	}

	minTemp, maxTemp = 1e10, -1e10
	prevT := 0.0
	for i, r := range readings {
		avgTemp += r.Temperature
		avgHumid += r.Humidity
		if r.Temperature < minTemp {
			minTemp = r.Temperature
		}
		if r.Temperature > maxTemp {
			maxTemp = r.Temperature
		}
		if i > 0 {
			if (prevT > 0 && r.Temperature <= 0) || (prevT <= 0 && r.Temperature > 0) {
				transitions++
			}
		}
		prevT = r.Temperature
	}
	n := float64(len(readings))
	avgTemp /= n
	avgHumid /= n
	tempRange = math.Max(maxTemp-minTemp, 15.0)
	transitions = int(math.Max(float64(transitions), float64(len(readings))*0.1))
	return
}

func calcCrackStats(readings []models.SensorReading) (avgCrack, maxCrack float64) {
	if len(readings) == 0 {
		return 0.1, 0.1
	}
	maxCrack = 0.0
	for _, r := range readings {
		avgCrack += r.MaxCrackWidth
		if r.MaxCrackWidth > maxCrack {
			maxCrack = r.MaxCrackWidth
		}
	}
	avgCrack /= float64(len(readings))
	if avgCrack < 0.01 {
		avgCrack = 0.01
	}
	if maxCrack < 0.01 {
		maxCrack = 0.01
	}
	return
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
