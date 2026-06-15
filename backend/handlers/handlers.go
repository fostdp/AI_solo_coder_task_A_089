package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"plankroad-backend/config"
	"plankroad-backend/models"
	"plankroad-backend/mqttclient"
	"plankroad-backend/repository"
	"plankroad-backend/simulation"
	"plankroad-backend/weathering"
)

type Handler struct {
	siteRepo    *repository.SiteRepo
	sensorRepo  *repository.SensorRepo
	simRepo     *repository.SimulationRepo
	weatherRepo *repository.WeatheringRepo
	alarmRepo   *repository.AlarmRepo
	femSolver   *simulation.Solver
	assessor    *weathering.Assessor
	mqttCli     *mqttclient.Client
	cfg         *config.Config
	siteNames   map[int]string
}

func NewHandler(cfg *config.Config, mqtt *mqttclient.Client) *Handler {
	return &Handler{
		siteRepo:    repository.NewSiteRepo(),
		sensorRepo:  repository.NewSensorRepo(),
		simRepo:     repository.NewSimulationRepo(),
		weatherRepo: repository.NewWeatheringRepo(),
		alarmRepo:   repository.NewAlarmRepo(),
		femSolver:   simulation.NewSolver(&cfg.FEM),
		assessor:    weathering.NewAssessor(&cfg.Weather),
		mqttCli:     mqtt,
		cfg:         cfg,
		siteNames:   make(map[int]string),
	}
}

func (h *Handler) LoadSiteNames(ctx context.Context) {
	sites, err := h.siteRepo.GetAll(ctx)
	if err == nil {
		for _, s := range sites {
			h.siteNames[s.SiteID] = s.SiteName
		}
	}
}

func (h *Handler) MQTTClient() *mqttclient.Client {
	return h.mqttCli
}

func (h *Handler) SiteNames() map[int]string {
	return h.siteNames
}

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Code: 0, Data: data})
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, Response{Code: code, Message: msg})
}

func (h *Handler) GetSites(c *gin.Context) {
	sites, err := h.siteRepo.GetAll(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, sites)
}

func (h *Handler) GetSite(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	site, err := h.siteRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		fail(c, http.StatusNotFound, "site not found")
		return
	}
	ok(c, site)
}

func (h *Handler) PostSensorReading(c *gin.Context) {
	var data models.SensorReading
	if err := c.ShouldBindJSON(&data); err != nil {
		fail(c, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}
	if data.Time.IsZero() {
		data.Time = time.Now()
	}

	if err := h.sensorRepo.InsertReading(c.Request.Context(), &data); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	go h.checkMQTTAlarms()

	c.JSON(http.StatusCreated, Response{Code: 0, Message: "received"})
}

func (h *Handler) PostBatchSensorReadings(c *gin.Context) {
	var readings []models.SensorReading
	if err := c.ShouldBindJSON(&readings); err != nil {
		fail(c, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}

	now := time.Now()
	for i := range readings {
		if readings[i].Time.IsZero() {
			readings[i].Time = now
		}
	}

	if err := h.sensorRepo.BatchInsert(c.Request.Context(), readings); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	go h.checkMQTTAlarms()

	ok(c, gin.H{"inserted": len(readings)})
}

func (h *Handler) GetSensorReadings(c *gin.Context) {
	siteID, _ := strconv.Atoi(c.Query("site_id"))
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "1000"))

	start := time.Now().Add(-time.Duration(hours) * time.Hour)
	end := time.Now()

	readings, err := h.sensorRepo.GetBySite(c.Request.Context(), siteID, start, end, limit)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, readings)
}

func (h *Handler) GetDailySummary(c *gin.Context) {
	siteID, _ := strconv.Atoi(c.Param("id"))
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))

	summary, err := h.sensorRepo.GetDailySummary(c.Request.Context(), siteID, days)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, summary)
}

func (h *Handler) RunSimulation(c *gin.Context) {
	siteID, _ := strconv.Atoi(c.Param("id"))
	ctx := c.Request.Context()

	site, err := h.siteRepo.GetByID(ctx, siteID)
	if err != nil {
		fail(c, http.StatusNotFound, "site not found")
		return
	}

	start := time.Now().Add(-48 * time.Hour)
	readings, err := h.sensorRepo.GetBySite(ctx, siteID, start, time.Now(), 500)
	if err != nil {
		readings = []models.SensorReading{}
	}

	sim, err := h.femSolver.Simulate(site, readings)
	if err != nil {
		fail(c, http.StatusInternalServerError, "simulation failed: "+err.Error())
		return
	}

	if err := h.simRepo.Save(ctx, sim); err != nil {
		fail(c, http.StatusInternalServerError, "save simulation failed: "+err.Error())
		return
	}

	go h.mqttCli.PublishSimulation(siteID, sim)

	ok(c, sim)
}

func (h *Handler) GetLatestSimulation(c *gin.Context) {
	siteID, _ := strconv.Atoi(c.Param("id"))
	sim, err := h.simRepo.GetLatest(c.Request.Context(), siteID)
	if err != nil {
		fail(c, http.StatusNotFound, "simulation not found")
		return
	}
	ok(c, sim)
}

func (h *Handler) RunWeathering(c *gin.Context) {
	siteID, _ := strconv.Atoi(c.Param("id"))
	ctx := c.Request.Context()

	site, err := h.siteRepo.GetByID(ctx, siteID)
	if err != nil {
		fail(c, http.StatusNotFound, "site not found")
		return
	}

	start := time.Now().Add(-720 * time.Hour)
	readings, _ := h.sensorRepo.GetBySite(ctx, siteID, start, time.Now(), 2000)
	sim, _ := h.simRepo.GetLatest(ctx, siteID)

	assessment := h.assessor.Assess(site, readings, sim)

	if err := h.weatherRepo.Save(ctx, assessment); err != nil {
		fail(c, http.StatusInternalServerError, "save assessment failed: "+err.Error())
		return
	}

	go h.mqttCli.PublishWeathering(siteID, assessment)

	ok(c, assessment)
}

func (h *Handler) GetLatestWeathering(c *gin.Context) {
	siteID, _ := strconv.Atoi(c.Param("id"))
	w, err := h.weatherRepo.GetLatest(c.Request.Context(), siteID)
	if err != nil {
		fail(c, http.StatusNotFound, "weathering assessment not found")
		return
	}
	ok(c, w)
}

func (h *Handler) GetAlarms(c *gin.Context) {
	siteID, _ := strconv.Atoi(c.DefaultQuery("site_id", "0"))
	alarms, err := h.alarmRepo.GetUnresolved(c.Request.Context(), siteID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, alarms)
}

func (h *Handler) AckAlarm(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	user := c.DefaultQuery("user", "system")
	if err := h.alarmRepo.Acknowledge(c.Request.Context(), id, user); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"acknowledged": true})
}

func (h *Handler) ResolveAlarm(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.alarmRepo.Resolve(c.Request.Context(), id); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"resolved": true})
}

func (h *Handler) GetThresholds(c *gin.Context) {
	siteID, _ := strconv.Atoi(c.Param("id"))
	th, err := h.alarmRepo.GetThresholds(c.Request.Context(), siteID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, th)
}

func (h *Handler) GetDashboard(c *gin.Context) {
	ctx := c.Request.Context()
	sites, _ := h.siteRepo.GetAll(ctx)

	dashboard := struct {
		TotalSites      int         `json:"total_sites"`
		ActiveAlarms    int         `json:"active_alarms"`
		CriticalAlarms  int         `json:"critical_alarms"`
		LastUpdate      time.Time   `json:"last_update"`
		SiteStatuses    []siteStatus `json:"site_statuses"`
	}{
		TotalSites: len(sites),
		LastUpdate: time.Now(),
	}

	for _, s := range sites {
		sim, _ := h.simRepo.GetLatest(ctx, s.SiteID)
		w, _ := h.weatherRepo.GetLatest(ctx, s.SiteID)
		alarms, _ := h.alarmRepo.GetUnresolved(ctx, s.SiteID)

		status := "HEALTHY"
		level := 1
		if w != nil {
			switch w.WeatheringGrade {
			case "SEVERE":
				status = "CRITICAL"
				level = 4
			case "SERIOUS":
				status = "DANGER"
				level = 3
			case "MODERATE":
				status = "WARNING"
				level = 2
			}
		}
		if sim != nil && sim.SafetyFactor < 1.2 {
			status = "CRITICAL"
			level = 4
		}
		if len(alarms) > 0 {
			for _, a := range alarms {
				if a.AlarmLevel == "CRITICAL" {
					status = "CRITICAL"
					level = 4
					dashboard.CriticalAlarms++
					break
				}
			}
			if level < 3 {
				status = "WARNING"
				level = 2
			}
		}

		dashboard.ActiveAlarms += len(alarms)

		st := siteStatus{
			SiteID:           s.SiteID,
			SiteName:         s.SiteName,
			Region:           s.Region,
			Status:           status,
			StatusLevel:      level,
			AlarmCount:       len(alarms),
			SafetyFactor:     -1,
			WeatheringGrade:  "",
			PredictedLifespan: -1,
		}
		if sim != nil {
			st.SafetyFactor = sim.SafetyFactor
		}
		if w != nil {
			st.WeatheringGrade = w.WeatheringGrade
			st.PredictedLifespan = w.RemainingLifespan
		}
		dashboard.SiteStatuses = append(dashboard.SiteStatuses, st)
	}

	ok(c, dashboard)
}

type siteStatus struct {
	SiteID           int     `json:"site_id"`
	SiteName         string  `json:"site_name"`
	Region           string  `json:"region"`
	Status           string  `json:"status"`
	StatusLevel      int     `json:"status_level"`
	AlarmCount       int     `json:"alarm_count"`
	SafetyFactor     float64 `json:"safety_factor"`
	WeatheringGrade  string  `json:"weathering_grade"`
	PredictedLifespan float64 `json:"predicted_lifespan_years"`
}

func (h *Handler) checkMQTTAlarms() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	published, err := h.mqttCli.ProcessPendingAlarms(ctx, h.siteNames)
	if err != nil {
		return
	}
	if published > 0 {
	}
}
