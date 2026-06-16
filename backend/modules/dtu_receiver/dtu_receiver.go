package dtu_receiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"

	"plankroad-backend/config"
	"plankroad-backend/models"
	"plankroad-backend/modules/bus"
	"plankroad-backend/repository"
)

type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s=%v: %s", e.Field, e.Value, e.Message)
}

type DTUReceiver struct {
	cfg          *config.Config
	bus          *bus.Bus
	logger       *log.Logger
	sensorRepo   *repository.SensorRepo
	siteRepo     *repository.SiteRepo
	siteNames    map[int]string
	mqttClient   mqtt.Client
	validSiteIDs map[int]bool
	metrics      struct {
		Received      uint64
		Validated     uint64
		Rejected      uint64
		Stored        uint64
		MQTTReceived  uint64
		HTTPReceived  uint64
	}
}

func New(cfg *config.Config, b *bus.Bus, logger *log.Logger) (*DTUReceiver, error) {
	if cfg == nil || b == nil {
		return nil, errors.New("config and bus must not be nil")
	}

	siteRepo := repository.NewSiteRepo()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sites, err := siteRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load sites: %w", err)
	}

	siteNames := make(map[int]string, len(sites))
	validIDs := make(map[int]bool, len(sites))
	for _, s := range sites {
		siteNames[s.SiteID] = s.SiteName
		validIDs[s.SiteID] = true
	}

	r := &DTUReceiver{
		cfg:          cfg,
		bus:          b,
		logger:       logger,
		sensorRepo:   repository.NewSensorRepo(),
		siteRepo:     siteRepo,
		siteNames:    siteNames,
		validSiteIDs: validIDs,
	}

	if err := r.subscribeMQTT(); err != nil {
		logger.Printf("[DTU] MQTT subscribe failed (continuing with HTTP only): %v", err)
	}

	r.setupSubscriptions()
	return r, nil
}

func (r *DTUReceiver) setupSubscriptions() {
	ctx := context.Background()
	r.bus.Subscribe(ctx, bus.EventCommandSimulate, "dtu-sim-cmd", 10,
		func(ctx context.Context, evt bus.Event) {
			r.logger.Printf("[DTU] Received simulation command for site %d", evt.SiteID)
		})
}

func (r *DTUReceiver) subscribeMQTT() error {
	if r.cfg.MQTT.Broker == "" {
		return errors.New("mqtt broker not configured")
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(r.cfg.MQTT.Broker)
	opts.SetClientID(fmt.Sprintf("dtu_receiver_%d", time.Now().Unix()))
	opts.SetUsername(r.cfg.MQTT.Username)
	opts.SetPassword(r.cfg.MQTT.Password)
	opts.SetAutoReconnect(true)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		r.logger.Printf("[DTU] MQTT connection lost: %v", err)
	})

	r.mqttClient = mqtt.NewClient(opts)
	if token := r.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("mqtt connect: %w", token.Error())
	}

	topic := "plankroad/data/+"
	token := r.mqttClient.Subscribe(topic, 1, r.handleMQTTMessage)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("mqtt subscribe %s: %w", topic, token.Error())
	}

	r.logger.Printf("[DTU] Subscribed to MQTT topic %s", topic)
	return nil
}

func (r *DTUReceiver) handleMQTTMessage(c mqtt.Client, msg mqtt.Message) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Printf("[DTU] Panic in MQTT handler: %v", rec)
		}
	}()

	var reading models.SensorReading
	if err := json.Unmarshal(msg.Payload(), &reading); err != nil {
		r.logger.Printf("[DTU] Failed to unmarshal MQTT payload: %v", err)
		r.metrics.Rejected++
		return
	}

	r.metrics.MQTTReceived++
	if err := r.validateAndStore(&reading, "mqtt"); err != nil {
		r.logger.Printf("[DTU] MQTT data rejected: %v", err)
	}
}

func (r *DTUReceiver) HandleHTTPBatch(c *gin.Context) {
	var batch []models.SensorReading
	if err := c.ShouldBindJSON(&batch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid batch: %v", err)})
		return
	}

	if len(batch) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty batch"})
		return
	}

	if len(batch) > 5000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "batch size exceeds 5000 limit"})
		return
	}

	r.metrics.HTTPReceived += uint64(len(batch))

	now := time.Now()
	validBatch := make([]models.SensorReading, 0, len(batch))
	for i := range batch {
		if batch[i].Time.IsZero() {
			batch[i].Time = now
		}
		if err := r.Validate(&batch[i]); err != nil {
			r.logger.Printf("[DTU] Batch item %d rejected: %v", i, err)
			r.metrics.Rejected++
			continue
		}
		validBatch = append(validBatch, batch[i])
	}

	if len(validBatch) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "all batch items failed validation"})
		return
	}

	if err := r.sensorRepo.BatchInsert(c.Request.Context(), validBatch); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("db insert: %v", err)})
		return
	}

	r.metrics.Stored += uint64(len(validBatch))
	r.metrics.Validated += uint64(len(validBatch))

	payload := &bus.SensorDataPayload{
		IsBatch: true,
		Batch:   validBatch,
		Source:  "http",
	}
	if err := r.bus.Publish(c.Request.Context(), bus.Event{
		Type:    bus.EventSensorDataValidated,
		SiteID:  validBatch[0].SiteID,
		Payload: payload,
	}); err != nil {
		r.logger.Printf("[DTU] Failed to publish validated event: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"received": len(batch),
		"accepted": len(validBatch),
		"rejected": len(batch) - len(validBatch),
	})
}

func (r *DTUReceiver) HandleHTTPSingle(c *gin.Context) {
	var reading models.SensorReading
	if err := c.ShouldBindJSON(&reading); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if reading.Time.IsZero() {
		reading.Time = time.Now()
	}

	r.metrics.HTTPReceived++

	if err := r.validateAndStore(&reading, "http"); err != nil {
		var verr *ValidationError
		if errors.As(err, &verr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   verr.Error(),
				"field":   verr.Field,
				"value":   verr.Value,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "timestamp": reading.Time})
}

func (r *DTUReceiver) validateAndStore(reading *models.SensorReading, source string) error {
	r.metrics.Received++

	if err := r.Validate(reading); err != nil {
		r.metrics.Rejected++
		return err
	}

	r.metrics.Validated++

	if err := r.sensorRepo.InsertReading(context.Background(), reading); err != nil {
		return fmt.Errorf("db insert: %w", err)
	}
	r.metrics.Stored++

	payload := &bus.SensorDataPayload{
		Reading: reading,
		IsBatch: false,
		Source:  source,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := r.bus.Publish(ctx, bus.Event{
		Type:    bus.EventSensorDataValidated,
		SiteID:  reading.SiteID,
		Payload: payload,
	}); err != nil {
		r.logger.Printf("[DTU] Warning: publish event failed: %v", err)
	}

	return nil
}

func (r *DTUReceiver) Validate(reading *models.SensorReading) error {
	if !r.validSiteIDs[reading.SiteID] {
		return &ValidationError{
			Field:   "SiteID",
			Value:   reading.SiteID,
			Message: "unknown site ID",
		}
	}

	if reading.BeamID < 1 || reading.BeamID > 1000 {
		return &ValidationError{
			Field:   "BeamID",
			Value:   reading.BeamID,
			Message: "beam ID out of range [1, 1000]",
		}
	}

	if reading.BeamStrainTop < -3000 || reading.BeamStrainTop > 3000 {
		return &ValidationError{
			Field:   "BeamStrainTop",
			Value:   reading.BeamStrainTop,
			Message: "strain out of range [-3000, 3000] microstrain",
		}
	}
	if reading.BeamStrainBottom < -3000 || reading.BeamStrainBottom > 3000 {
		return &ValidationError{
			Field:   "BeamStrainBottom",
			Value:   reading.BeamStrainBottom,
			Message: "strain out of range [-3000, 3000] microstrain",
		}
	}
	if reading.BeamStrainSide < -3000 || reading.BeamStrainSide > 3000 {
		return &ValidationError{
			Field:   "BeamStrainSide",
			Value:   reading.BeamStrainSide,
			Message: "strain out of range [-3000, 3000] microstrain",
		}
	}

	for i, v := range []float64{reading.RockCrackWidth1, reading.RockCrackWidth2, reading.RockCrackWidth3} {
		if v < 0 || v > 200 {
			return &ValidationError{
				Field:   fmt.Sprintf("RockCrackWidth%d", i+1),
				Value:   v,
				Message: "crack width out of range [0, 200] mm",
			}
		}
	}

	if reading.Temperature < -40 || reading.Temperature > 60 {
		return &ValidationError{
			Field:   "Temperature",
			Value:   reading.Temperature,
			Message: "temperature out of range [-40, 60] °C",
		}
	}

	if reading.Humidity < 0 || reading.Humidity > 100 {
		return &ValidationError{
			Field:   "Humidity",
			Value:   reading.Humidity,
			Message: "humidity out of range [0, 100] %RH",
		}
	}

	if reading.Time.After(time.Now().Add(24 * time.Hour)) {
		return &ValidationError{
			Field:   "Time",
			Value:   reading.Time,
			Message: "timestamp in the future (>24h)",
		}
	}

	return nil
}

func (r *DTUReceiver) GetSiteNames() map[int]string {
	return r.siteNames
}

func (r *DTUReceiver) GetSiteRepo() *repository.SiteRepo {
	return r.siteRepo
}

func (r *DTUReceiver) GetSensorRepo() *repository.SensorRepo {
	return r.sensorRepo
}

func (r *DTUReceiver) GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"received":     r.metrics.Received,
		"validated":    r.metrics.Validated,
		"rejected":     r.metrics.Rejected,
		"stored":       r.metrics.Stored,
		"mqtt_received": r.metrics.MQTTReceived,
		"http_received": r.metrics.HTTPReceived,
	}
}

func (r *DTUReceiver) Close() {
	if r.mqttClient != nil && r.mqttClient.IsConnected() {
		r.mqttClient.Disconnect(250)
		r.logger.Println("[DTU] MQTT client disconnected")
	}
}
