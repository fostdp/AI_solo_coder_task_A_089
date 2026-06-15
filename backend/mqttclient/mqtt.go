package mqttclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"plankroad-backend/config"
	"plankroad-backend/models"
	"plankroad-backend/repository"
)

type Client struct {
	client    mqtt.Client
	cfg       *config.MQTTConfig
	alarmRepo *repository.AlarmRepo
	sensorRepo *repository.SensorRepo
}

type AlarmMQTTMessage struct {
	AlarmID        int64     `json:"alarm_id"`
	Time           time.Time `json:"time"`
	SiteID         int       `json:"site_id"`
	SiteName       string    `json:"site_name,omitempty"`
	BeamID         int       `json:"beam_id,omitempty"`
	AlarmType      string    `json:"alarm_type"`
	AlarmLevel     string    `json:"alarm_level"`
	MetricName     string    `json:"metric_name,omitempty"`
	CurrentValue   float64   `json:"current_value,omitempty"`
	ThresholdValue float64   `json:"threshold_value,omitempty"`
	Description    string    `json:"description"`
	Severity       int       `json:"severity"`
}

type SensorMQTTMessage struct {
	Time             time.Time `json:"time"`
	SiteID           int       `json:"site_id"`
	BeamID           int       `json:"beam_id"`
	BeamStrainTop    float64   `json:"beam_strain_top"`
	BeamStrainBottom float64   `json:"beam_strain_bottom"`
	BeamStrainSide   float64   `json:"beam_strain_side"`
	RockCrackWidth1  float64   `json:"rock_crack_width_1"`
	RockCrackWidth2  float64   `json:"rock_crack_width_2"`
	RockCrackWidth3  float64   `json:"rock_crack_width_3"`
	Temperature      float64   `json:"temperature"`
	Humidity         float64   `json:"humidity"`
	Rainfall         float64   `json:"rainfall"`
}

func NewClient(cfg *config.MQTTConfig, alarmRepo *repository.AlarmRepo, sensorRepo *repository.SensorRepo) *Client {
	return &Client{
		cfg:        cfg,
		alarmRepo:  alarmRepo,
		sensorRepo: sensorRepo,
	}
}

func (c *Client) Connect() error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(c.cfg.Broker)
	opts.SetClientID(c.cfg.ClientID)
	opts.SetUsername(c.cfg.Username)
	opts.SetPassword(c.cfg.Password)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)

	opts.OnConnect = func(client mqtt.Client) {
		log.Printf("MQTT connected to %s", c.cfg.Broker)
		c.subscribe()
	}

	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
	}

	opts.OnReconnecting = func(client mqtt.Client, opts *mqtt.ClientOptions) {
		log.Printf("MQTT reconnecting...")
	}

	c.client = mqtt.NewClient(opts)

	token := c.client.Connect()
	token.WaitTimeout(30 * time.Second)
	if token.Error() != nil {
		return fmt.Errorf("mqtt connect: %w", token.Error())
	}

	log.Printf("MQTT client initialized: %s", c.cfg.ClientID)
	return nil
}

func (c *Client) subscribe() {
	dataTopic := c.cfg.DataTopic
	token := c.client.Subscribe(dataTopic, 1, c.handleSensorData)
	token.WaitTimeout(10 * time.Second)
	if token.Error() != nil {
		log.Printf("MQTT subscribe %s error: %v", dataTopic, token.Error())
	} else {
		log.Printf("MQTT subscribed: %s", dataTopic)
	}
}

func (c *Client) handleSensorData(client mqtt.Client, msg mqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in MQTT handler: %v", r)
		}
	}()

	var data SensorMQTTMessage
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("Invalid sensor MQTT payload: %v", err)
		return
	}

	reading := models.SensorReading{
		Time:             data.Time,
		SiteID:           data.SiteID,
		BeamID:           data.BeamID,
		BeamStrainTop:    data.BeamStrainTop,
		BeamStrainBottom: data.BeamStrainBottom,
		BeamStrainSide:   data.BeamStrainSide,
		RockCrackWidth1:  data.RockCrackWidth1,
		RockCrackWidth2:  data.RockCrackWidth2,
		RockCrackWidth3:  data.RockCrackWidth3,
		Temperature:      data.Temperature,
		Humidity:         data.Humidity,
		Rainfall:         data.Rainfall,
	}

	if reading.Time.IsZero() {
		reading.Time = time.Now()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.sensorRepo.InsertReading(ctx, &reading); err != nil {
		log.Printf("Insert sensor reading error: site=%d beam=%d err=%v",
			data.SiteID, data.BeamID, err)
	}
}

func (c *Client) PublishAlarm(alarm *models.AlarmEvent, siteName string) error {
	if !c.client.IsConnected() {
		return fmt.Errorf("mqtt not connected")
	}

	severity := 1
	if alarm.AlarmLevel == "WARNING" {
		severity = 2
	} else if alarm.AlarmLevel == "CRITICAL" {
		severity = 3
	}

	msg := AlarmMQTTMessage{
		AlarmID:        alarm.AlarmID,
		Time:           alarm.Time,
		SiteID:         alarm.SiteID,
		SiteName:       siteName,
		BeamID:         alarm.BeamID,
		AlarmType:      alarm.AlarmType,
		AlarmLevel:     alarm.AlarmLevel,
		MetricName:     alarm.MetricName,
		CurrentValue:   alarm.CurrentValue,
		ThresholdValue: alarm.ThresholdValue,
		Description:    alarm.Description,
		Severity:       severity,
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal alarm: %w", err)
	}

	topic := fmt.Sprintf("plankroad/alarm/%d/%s", alarm.SiteID, alarm.AlarmType)
	qos := byte(2)
	retained := false

	token := c.client.Publish(topic, qos, retained, payload)
	token.WaitTimeout(10 * time.Second)
	if token.Error() != nil {
		return fmt.Errorf("publish alarm %d: %w", alarm.AlarmID, token.Error())
	}

	return nil
}

func (c *Client) ProcessPendingAlarms(ctx context.Context, siteNames map[int]string) (int, error) {
	alarms, err := c.alarmRepo.GetUnpublished(ctx)
	if err != nil {
		return 0, fmt.Errorf("get unpublished alarms: %w", err)
	}

	published := 0
	for _, alarm := range alarms {
		siteName := siteNames[alarm.SiteID]
		if err := c.PublishAlarm(&alarm, siteName); err != nil {
			log.Printf("Publish alarm %d error: %v", alarm.AlarmID, err)
			continue
		}

		if err := c.alarmRepo.MarkMQTTPublished(ctx, alarm.AlarmID); err != nil {
			log.Printf("Mark alarm %d published error: %v", alarm.AlarmID, err)
		}
		published++
	}

	return published, nil
}

func (c *Client) PublishSimulation(siteID int, sim *models.StructuralSimulation) error {
	if !c.client.IsConnected() {
		return nil
	}

	topic := fmt.Sprintf("plankroad/simulation/%d", siteID)
	payload, _ := json.Marshal(struct {
		SiteID          int       `json:"site_id"`
		SimTime         time.Time `json:"sim_time"`
		MaxWoodStress   float64   `json:"max_wood_stress_mpa"`
		MaxRockStress   float64   `json:"max_rock_stress_mpa"`
		MaxDeflectionMM float64   `json:"max_deflection_mm"`
		SafetyFactor    float64   `json:"safety_factor"`
	}{
		SiteID:          siteID,
		SimTime:         sim.SimTime,
		MaxWoodStress:   sim.MaxWoodStress,
		MaxRockStress:   sim.MaxRockStress,
		MaxDeflectionMM: sim.MaxDeflectionMM,
		SafetyFactor:    sim.SafetyFactor,
	})

	c.client.Publish(topic, 0, false, payload)
	return nil
}

func (c *Client) PublishWeathering(siteID int, w *models.WeatheringAssessment) error {
	if !c.client.IsConnected() {
		return nil
	}

	topic := fmt.Sprintf("plankroad/weathering/%d", siteID)
	payload, _ := json.Marshal(struct {
		SiteID              int       `json:"site_id"`
		AssessTime          time.Time `json:"assess_time"`
		FreezeThawCycles    int       `json:"freeze_thaw_cycles"`
		CurrentCrackDepth   float64   `json:"current_crack_depth_mm"`
		CrackPropagationRate float64  `json:"crack_propagation_rate_mm_year"`
		WeatheringGrade     string    `json:"weathering_grade"`
		PredictedLifespan   float64   `json:"predicted_lifespan_years"`
		RemainingLifespan   float64   `json:"remaining_lifespan_years"`
		Confidence          float64   `json:"confidence"`
	}{
		SiteID:              siteID,
		AssessTime:          w.AssessTime,
		FreezeThawCycles:    w.FreezeThawCycles,
		CurrentCrackDepth:   w.CurrentCrackDepth,
		CrackPropagationRate: w.CrackPropagationRate,
		WeatheringGrade:     w.WeatheringGrade,
		PredictedLifespan:   w.PredictedLifespan,
		RemainingLifespan:   w.RemainingLifespan,
		Confidence:          w.Confidence,
	})

	c.client.Publish(topic, 0, false, payload)
	return nil
}

func (c *Client) Disconnect() {
	if c.client != nil {
		c.client.Disconnect(250)
		log.Println("MQTT client disconnected")
	}
}

func (c *Client) IsConnected() bool {
	return c.client != nil && c.client.IsConnected()
}
