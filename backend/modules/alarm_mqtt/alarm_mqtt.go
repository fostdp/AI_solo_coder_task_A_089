package alarm_mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"plankroad-backend/config"
	"plankroad-backend/models"
	"plankroad-backend/modules/bus"
	"plankroad-backend/repository"
)

type AlarmMQTT struct {
	cfg             *config.Config
	bus               *bus.Bus
	logger            *log.Logger
	alarmRepo         *repository.AlarmRepo
	siteRepo          *repository.SiteRepo
	sensorRepo        *repository.SensorRepo
	simRepo           *repository.SimulationRepo
	siteNames         map[int]string
	mqttClient         mqtt.Client
	alarmMu           sync.Mutex
	alarmTicker        *time.Ticker
	alarmTickerStop    chan struct{}
	metrics             struct {
		AlarmsRaised      uint64
		AlarmsPublished    uint64
		AlarmsAcknowledged uint64
		AlarmsResolved     uint64
		MessagesSent      uint64
		Retries           uint64
		Failures          uint64
	}
}

type AlarmMQTTMessage struct {
	SiteID        int    `json:"site_id"`
	SiteName      string `json:"site_name"`
	BeamID        int    `json:"beam_id"`
	AlarmType       string `json:"alarm_type"`
	AlarmLevel      string `json:"alarm_level"`
	MetricName      string `json:"metric_name"`
	CurrentValue    float64 `json:"current_value"`
	ThresholdValue  float64 `json:"threshold_value"`
	Description     string `json:"description"`
	Severity        int    `json:"severity"`
	Timestamp       string `json:"timestamp"`
	AlarmID         int    `json:"alarm_id"`
}

func New(cfg *config.Config, b *bus.Bus, logger *log.Logger) (*AlarmMQTT, error) {
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

	a := &AlarmMQTT{
		cfg:         cfg,
		bus:         b,
		logger:      logger,
		alarmRepo:   repository.NewAlarmRepo(),
		siteRepo:    siteRepo,
		sensorRepo:  repository.NewSensorRepo(),
		simRepo:     repository.NewSimulationRepo(),
		siteNames:   siteNames,
		alarmTickerStop: make(chan struct{}),
	}

	if err := a.connectMQTT(); err != nil {
		logger.Printf("[ALARM] MQTT connect failed (continuing with API only): %v", err)
	}

	a.setupSubscriptions()
	return a, nil
}

func (a *AlarmMQTT) connectMQTT() error {
	if a.cfg.MQTT.Broker == "" {
		return fmt.Errorf("mqtt config is nil")
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(a.cfg.MQTT.Broker)
	opts.SetClientID(fmt.Sprintf("alarm_mqtt_%d", time.Now().Unix()))
	opts.SetUsername(a.cfg.MQTT.Username)
	opts.SetPassword(a.cfg.MQTT.Password)
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		a.logger.Printf("[ALARM] MQTT connection lost: %v", err)
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		a.logger.Println("[ALARM] MQTT connected")
	})

	a.mqttClient = mqtt.NewClient(opts)
	if token := a.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("mqtt connect: %w", token.Error())
	}

	return nil
}

func (a *AlarmMQTT) setupSubscriptions() {
	ctx := context.Background()

	a.bus.Subscribe(ctx, bus.EventSensorDataValidated, "alarm-data", 100,
		func(ctx context.Context, evt bus.Event) {
			p, ok := evt.PayloadAsSensorData()
			if !ok {
				return
			}

			a.logger.Printf("[ALARM] Validated data received for site %d", evt.SiteID)

			if p.IsBatch && len(p.Batch) > 0 {
				go a.checkBatchAlarms(p.Batch)
			} else if p.Reading != nil {
				go a.checkReadingAlarms(p.Reading)
			}
		})

	a.bus.Subscribe(ctx, bus.EventWeatheringReady, "alarm-weathering", 20,
		func(ctx context.Context, evt bus.Event) {
			p, ok := evt.PayloadAsWeathering()
			if !ok {
				return
			}
			a.evaluateWeatheringAlarms(p.Assessment)
		})

	a.bus.Subscribe(ctx, bus.EventStructuralSimReady, "alarm-sim", 20,
		func(ctx context.Context, evt bus.Event) {
			p, ok := evt.PayloadAsSimulation()
			if !ok {
				return
			}
			a.evaluateStructuralAlarms(p.Simulation)
		})
}

func (a *AlarmMQTT) checkBatchAlarms(batch []models.SensorReading) {
	for _, r := range batch {
		if r.StrainAlarm || r.CrackAlarm {
			a.processAlarm(r)
		}
	}
}

func (a *AlarmMQTT) checkReadingAlarms(r *models.SensorReading) {
	if r.StrainAlarm || r.CrackAlarm {
		a.processAlarm(*r)
	}
}

func (a *AlarmMQTT) processAlarm(r models.SensorReading) {
	a.alarmMu.Lock()
	defer a.alarmMu.Unlock()

	a.metrics.AlarmsRaised++

	alarmType := ""
	metricName := ""
	currentValue := 0.0
	threshold := 0.0
	severity := 1

	avgStrain := (r.BeamStrainTop + r.BeamStrainBottom + r.BeamStrainSide) / 3.0
	maxCrack := math.Max(math.Max(r.RockCrackWidth1, r.RockCrackWidth2), r.RockCrackWidth3)

	if r.StrainAlarm {
		alarmType = "STRAIN"
		metricName = "梁孔应变均值"
		currentValue = avgStrain
		threshold = a.cfg.Alarm.StrainThreshold
		if math.Abs(avgStrain) > a.cfg.Alarm.StrainCritical {
			severity = 3
		} else {
			severity = 2
		}
	}

	if r.CrackAlarm {
		alarmType = "CRACK"
		metricName = "岩体裂隙宽度"
		currentValue = maxCrack
		threshold = a.cfg.Alarm.CrackThreshold
		if maxCrack > a.cfg.Alarm.CrackCritical {
			severity = 3
		} else {
			severity = 2
		}
	}

	level := "WARNING"
	if severity == 3 {
		level = "CRITICAL"
	} else if severity == 1 {
		level = "INFO"
	}

	event := &models.AlarmEvent{
		SiteID:         r.SiteID,
		BeamID:         r.BeamID,
		AlarmType:       alarmType,
		AlarmLevel:      level,
		MetricName:       metricName,
		CurrentValue:    currentValue,
		ThresholdValue:  threshold,
		Description:      fmt.Sprintf("%s=%.3f 超过阈值%.1f",
			metricName, currentValue, threshold),
		Severity:     severity,
		Time:      r.Time,
		Acknowledged: false,
		Resolved:     false,
	}

	ctx := context.Background()
	if err := a.alarmRepo.Insert(ctx, event); err != nil {
		a.logger.Printf("[ALARM] Failed to insert alarm: %v", err)
		a.metrics.Failures++
		return
	}

	a.publishAlarm(event)
}

func (a *AlarmMQTT) evaluateWeatheringAlarms(wa *models.WeatheringAssessment) {
	if wa == nil {
		return
	}

	gradeSeverity := map[string]int{
		"SEVERE": 3, "SERIOUS": 3, "MODERATE": 2, "MILD": 1, "SLIGHT": 0,
	}

	if wa.WeatheringGrade == "SEVERE" || wa.WeatheringGrade == "SERIOUS" {
		event := &models.AlarmEvent{
			SiteID:         wa.SiteID,
			BeamID:         0,
			AlarmType:       "WEATHERING",
			AlarmLevel:      "WARNING",
			MetricName:       "风化等级",
			CurrentValue:    wa.CurrentCrackDepth,
			ThresholdValue:  a.cfg.Alarm.CrackThreshold,
			Description:     fmt.Sprintf("风化等级=%s, 剩余寿命=%.1f年",
				wa.WeatheringGrade, wa.RemainingLifespan),
			Severity:     gradeSeverity[wa.WeatheringGrade],
			Time:    wa.AssessTime,
		}

		a.alarmMu.Lock()
		a.metrics.AlarmsRaised++
		ctx := context.Background()
		if err := a.alarmRepo.Insert(ctx, event); err != nil {
			a.logger.Printf("[ALARM] Failed to insert weathering alarm: %v", err)
			a.metrics.Failures++
		} else {
			a.publishAlarm(event)
		}
		a.alarmMu.Unlock()
	}
}

func (a *AlarmMQTT) evaluateStructuralAlarms(sim *models.StructuralSimulation) {
	if sim == nil {
		return
	}

	if sim.SafetyFactor < 1.2 {
		level := "WARNING"
		severity := 2
		if sim.SafetyFactor < 1.0 {
			level = "CRITICAL"
			severity = 3
		}

		event := &models.AlarmEvent{
			SiteID:         sim.SiteID,
			BeamID:         0,
			AlarmType:       "STRUCTURAL",
			AlarmLevel:      level,
			MetricName:       "结构安全系数",
			CurrentValue:    sim.SafetyFactor,
			ThresholdValue:  1.2,
			Description:     fmt.Sprintf("安全系数=%.3f, 最大应力=%.2f MPa",
				sim.SafetyFactor, math.Max(sim.MaxWoodStress, sim.MaxRockStress)),
			Severity:     severity,
			Time:    sim.SimTime,
		}

		a.alarmMu.Lock()
		a.metrics.AlarmsRaised++
		ctx := context.Background()
		if err := a.alarmRepo.Insert(ctx, event); err != nil {
			a.logger.Printf("[ALARM] Failed to insert structural alarm: %v", err)
			a.metrics.Failures++
		} else {
			a.publishAlarm(event)
		}
		a.alarmMu.Unlock()
	}
}

func (a *AlarmMQTT) publishAlarm(alarm *models.AlarmEvent) {
	if alarm == nil {
		return
	}

	siteName := ""
	if name, ok := a.siteNames[alarm.SiteID]; ok {
		siteName = name
	}

	msg := &AlarmMQTTMessage{
		SiteID:        alarm.SiteID,
		SiteName:      siteName,
		BeamID:        alarm.BeamID,
		AlarmType:       alarm.AlarmType,
		AlarmLevel:      alarm.AlarmLevel,
		MetricName:      alarm.MetricName,
		CurrentValue:    alarm.CurrentValue,
		ThresholdValue:  alarm.ThresholdValue,
		Description:     alarm.Description,
		Severity:        alarm.Severity,
		Timestamp:       alarm.Time.Format(time.RFC3339),
	AlarmID:         int(alarm.AlarmID),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		a.logger.Printf("[ALARM] Marshal alarm message failed: %v", err)
		a.metrics.Failures++
		return
	}

	a.logger.Printf("[ALARM] Publishing %s alarm site=%d beam=%d sev=%d",
		alarm.AlarmType, alarm.SiteID, alarm.BeamID, alarm.Severity)

	if a.mqttClient != nil && a.mqttClient.IsConnected() {
		topic := fmt.Sprintf("plankroad/alarm/%d/%s", alarm.SiteID, alarm.AlarmType)
		qos := byte(2)
		ctx := context.Background()

		for attempt := 0; attempt < 3; attempt++ {
			token := a.mqttClient.Publish(topic, qos, false, data)
			if token.WaitTimeout(5 * time.Second) {
				if token.Error() == nil {
					a.alarmRepo.MarkMQTTPublished(ctx, alarm.AlarmID)
					a.metrics.AlarmsPublished++
					a.metrics.MessagesSent++
					break
				}
				a.logger.Printf("[ALARM] Publish failed (attempt %d): %v",
					attempt+1, token.Error())
				a.metrics.Retries++
			}
			if attempt < 2 {
				time.Sleep(2 * time.Second)
			}
		}
	} else {
		a.logger.Printf("[ALARM] MQTT not connected, alarm stored in DB")
	}

	a.bus.Publish(context.Background(), bus.Event{
		Type:    bus.EventAlarmRaised,
		SiteID:  alarm.SiteID,
		Payload: &bus.AlarmPayload{Alarm: alarm, SiteName: siteName},
	})
}

func (a *AlarmMQTT) PublishSimulation(sim *models.StructuralSimulation) error {
	if sim == nil {
		return nil
	}

	if a.mqttClient == nil || !a.mqttClient.IsConnected() {
		return fmt.Errorf("mqtt not connected")
	}

	data, err := json.Marshal(sim)
	if err != nil {
		return fmt.Errorf("marshal sim: %w", err)
	}

	topic := fmt.Sprintf("plankroad/simulation/%d", sim.SiteID)
	token := a.mqttClient.Publish(topic, byte(1), false, data)
	if token.WaitTimeout(5 * time.Second) && token.Error() != nil {
		return fmt.Errorf("publish: %w", token.Error())
	}
	a.logger.Printf("[ALARM] Published simulation for site %d", sim.SiteID)
	return nil
}

func (a *AlarmMQTT) PublishWeathering(wa *models.WeatheringAssessment) error {
	if wa == nil {
		return nil
	}

	if a.mqttClient == nil || !a.mqttClient.IsConnected() {
		return fmt.Errorf("mqtt not connected")
	}

	data, err := json.Marshal(wa)
	if err != nil {
		return fmt.Errorf("marshal weathering: %w", err)
	}

	topic := fmt.Sprintf("plankroad/weathering/%d", wa.SiteID)
	token := a.mqttClient.Publish(topic, byte(1), false, data)
	if token.WaitTimeout(5 * time.Second) && token.Error() != nil {
		return fmt.Errorf("publish: %w", token.Error())
	}
	a.logger.Printf("[ALARM] Published weathering for site %d", wa.SiteID)
	return nil
}

func (a *AlarmMQTT) ProcessPendingAlarms(ctx context.Context) error {
	alarms, err := a.alarmRepo.GetUnpublished(ctx)
	if err != nil {
		return fmt.Errorf("get unpublished: %w", err)
	}

	if len(alarms) == 0 {
		return nil
	}

	a.logger.Printf("[ALARM] Processing %d pending alarms", len(alarms))

	for _, alarm := range alarms {
		alarmCopy := alarm
		a.publishAlarm(&alarmCopy)
	}

	return nil
}

func (a *AlarmMQTT) StartAlarmProcessor(interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	a.alarmTicker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-a.alarmTicker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				if err := a.ProcessPendingAlarms(ctx); err != nil {
					a.logger.Printf("[ALARM] Process pending failed: %v", err)
				}
				cancel()
			case <-a.alarmTickerStop:
				a.logger.Println("[ALARM] Alarm processor stopped")
				return
			}
		}
	}()

	a.logger.Printf("[ALARM] Alarm processor started (interval=%v)", interval)
}

func (a *AlarmMQTT) AckAlarm(alarmID int, user string) error {
	ctx := context.Background()
	if err := a.alarmRepo.Acknowledge(ctx, int64(alarmID), user); err != nil {
		return fmt.Errorf("ack alarm: %w", err)
	}
	a.metrics.AlarmsAcknowledged++
	a.logger.Printf("[ALARM] Alarm %d acknowledged by %s", alarmID, user)
	return nil
}

func (a *AlarmMQTT) ResolveAlarm(alarmID int, user string, comment string) error {
	ctx := context.Background()
	if err := a.alarmRepo.Resolve(ctx, int64(alarmID)); err != nil {
		return fmt.Errorf("resolve alarm: %w", err)
	}
	a.metrics.AlarmsResolved++
	a.logger.Printf("[ALARM] Alarm %d resolved by %s: %s", alarmID, user, comment)
	return nil
}

func (a *AlarmMQTT) GetAlarmRepo() *repository.AlarmRepo {
	return a.alarmRepo
}

func (a *AlarmMQTT) GetSiteRepo() *repository.SiteRepo {
	return a.siteRepo
}

func (a *AlarmMQTT) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"alarms_raised":      a.metrics.AlarmsRaised,
		"alarms_published": a.metrics.AlarmsPublished,
		"alarms_ack":       a.metrics.AlarmsAcknowledged,
		"alarms_resolved":  a.metrics.AlarmsResolved,
		"messages_sent":     a.metrics.MessagesSent,
		"retries":           a.metrics.Retries,
		"failures":          a.metrics.Failures,
	}
}

func (a *AlarmMQTT) Close() {
	if a.alarmTicker != nil {
		a.alarmTicker.Stop()
		close(a.alarmTickerStop)
	}
	if a.mqttClient != nil && a.mqttClient.IsConnected() {
		a.mqttClient.Disconnect(250)
		a.logger.Println("[ALARM] MQTT client disconnected")
	}
}
