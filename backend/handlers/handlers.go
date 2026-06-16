package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"plankroad-backend/config"
	"plankroad-backend/modules/alarm_mqtt"
	"plankroad-backend/modules/bus"
	"plankroad-backend/modules/dtu_receiver"
	"plankroad-backend/modules/structural_simulator"
	"plankroad-backend/modules/weathering_evaluator"
)

type Handler struct {
	cfg                *config.Config
	bus                *bus.Bus
	dtuReceiver        *dtu_receiver.DTUReceiver
	structuralSim      *structural_simulator.StructuralSimulator
	weatheringEval     *weathering_evaluator.WeatheringEvaluator
	alarmMQTT          *alarm_mqtt.AlarmMQTT
}

func New(cfg *config.Config, b *bus.Bus,
	dtu *dtu_receiver.DTUReceiver,
	sim *structural_simulator.StructuralSimulator,
	we *weathering_evaluator.WeatheringEvaluator,
	al *alarm_mqtt.AlarmMQTT) *Handler {

	return &Handler{
		cfg:            cfg,
		bus:            b,
		dtuReceiver:    dtu,
		structuralSim:  sim,
		weatheringEval: we,
		alarmMQTT:      al,
	}
}

func (h *Handler) GetSites(c *gin.Context) {
	ctx := c.Request.Context()
	sites, err := h.dtuReceiver.GetSiteRepo().GetAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sites)
}

func (h *Handler) GetSite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	ctx := c.Request.Context()
	site, err := h.dtuReceiver.GetSiteRepo().GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, site)
}

func (h *Handler) PostBatchSensorReadings(c *gin.Context) {
	h.dtuReceiver.HandleHTTPBatch(c)
}

func (h *Handler) PostSingleSensorReading(c *gin.Context) {
	h.dtuReceiver.HandleHTTPSingle(c)
}

func (h *Handler) RunSimulation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	sim, err := h.structuralSim.RunForSite(id, "api")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, sim)
}

func (h *Handler) RunWeathering(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	wa, err := h.weatheringEval.RunForSite(id, "api")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, wa)
}

func (h *Handler) GetRecentReadings(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	ctx := c.Request.Context()
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(hours) * time.Hour)
	readings, err := h.dtuReceiver.GetSensorRepo().GetBySite(ctx, id, startTime, endTime, 1000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, readings)
}

func (h *Handler) GetLatestSimulation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	ctx := c.Request.Context()
	sim, err := h.structuralSim.GetSimRepo().GetLatest(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sim)
}

func (h *Handler) GetLatestWeathering(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	ctx := c.Request.Context()
	wa, err := h.weatheringEval.GetWeatherRepo().GetLatest(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, wa)
}

func (h *Handler) GetAlarms(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	siteID, _ := strconv.Atoi(c.DefaultQuery("site_id", "0"))
	ctx := c.Request.Context()

	alarms, err := h.alarmMQTT.GetAlarmRepo().GetUnresolved(ctx, siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(alarms) > limit {
		alarms = alarms[:limit]
	}
	c.JSON(http.StatusOK, alarms)
}

func (h *Handler) AckAlarm(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alarm id"})
		return
	}

	var body struct {
		User string `json:"user"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.User = "system"
	}

	if err := h.alarmMQTT.AckAlarm(id, body.User); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "acknowledged"})
}

func (h *Handler) ResolveAlarm(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alarm id"})
		return
	}

	var body struct {
		User    string `json:"user"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.User = "system"
	}

	if err := h.alarmMQTT.ResolveAlarm(id, body.User, body.Comment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resolved"})
}

func (h *Handler) GetDashboard(c *gin.Context) {
	ctx := c.Request.Context()
	sites, err := h.dtuReceiver.GetSiteRepo().GetAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	statuses := make([]map[string]interface{}, 0, len(sites))
	for _, s := range sites {
		sim, _ := h.structuralSim.GetSimRepo().GetLatest(ctx, s.SiteID)
		wa, _ := h.weatheringEval.GetWeatherRepo().GetLatest(ctx, s.SiteID)
		alarms, _ := h.alarmMQTT.GetAlarmRepo().GetUnresolved(ctx, s.SiteID)

		status := "HEALTHY"
		if sim != nil {
			if sim.SafetyFactor < 1.0 {
				status = "CRITICAL"
			} else if sim.SafetyFactor < 1.2 {
				status = "WARNING"
			}
		}
		if wa != nil && (wa.WeatheringGrade == "SEVERE" || wa.WeatheringGrade == "SERIOUS") {
			if status != "CRITICAL" {
				status = "DANGER"
			}
		}
		if len(alarms) > 0 {
			for _, a := range alarms {
				if a.Severity == 3 && status != "CRITICAL" {
					status = "DANGER"
				}
			}
		}

		statuses = append(statuses, map[string]interface{}{
			"site_id":    s.SiteID,
			"name":       s.SiteName,
			"status":     status,
			"simulation": sim,
			"weathering": wa,
			"alarms_cnt": len(alarms),
			"updated_at": time.Now(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"sites":       statuses,
		"module_metrics": map[string]interface{}{
			"dtu_receiver":         h.dtuReceiver.GetMetrics(),
			"structural_simulator": h.structuralSim.GetMetrics(),
			"weathering_evaluator": h.weatheringEval.GetMetrics(),
			"alarm_mqtt":           h.alarmMQTT.GetMetrics(),
			"bus":                  h.bus.GetMetrics(),
		},
	})
}

func (h *Handler) GetDailySummary(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	ctx := c.Request.Context()

	summary, err := h.dtuReceiver.GetSensorRepo().GetDailySummary(ctx, id, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (h *Handler) TriggerSimAll(c *gin.Context) {
	go func() {
		h.structuralSim.RunAll("api")
	}()
	c.JSON(http.StatusAccepted, gin.H{"status": "scheduled", "count": 10})
}

func (h *Handler) TriggerWeatheringAll(c *gin.Context) {
	go func() {
		h.weatheringEval.RunAll("api")
	}()
	c.JSON(http.StatusAccepted, gin.H{"status": "scheduled", "count": 10})
}

func (h *Handler) DTUReceiver() *dtu_receiver.DTUReceiver {
	return h.dtuReceiver
}

func (h *Handler) StructuralSimulator() *structural_simulator.StructuralSimulator {
	return h.structuralSim
}

func (h *Handler) WeatheringEvaluator() *weathering_evaluator.WeatheringEvaluator {
	return h.weatheringEval
}

func (h *Handler) AlarmMQTT() *alarm_mqtt.AlarmMQTT {
	return h.alarmMQTT
}

func (h *Handler) SiteNames() map[int]string {
	return h.dtuReceiver.GetSiteNames()
}

func (h *Handler) checkMQTTAlarms() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.alarmMQTT.ProcessPendingAlarms(ctx)
}
