package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"plankroad-backend/config"
	"plankroad-backend/database"
	"plankroad-backend/handlers"
	"plankroad-backend/mqttclient"
	"plankroad-backend/repository"
	"plankroad-backend/simulation"
	"plankroad-backend/weathering"
)

func main() {
	cfg := config.Load()

	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	if err := database.Init(&cfg.Database); err != nil {
		log.Fatalf("Database init failed: %v", err)
	}
	defer database.Close()

	alarmRepo := repository.NewAlarmRepo()
	sensorRepo := repository.NewSensorRepo()

	mqttCli := mqttclient.NewClient(&cfg.MQTT, alarmRepo, sensorRepo)
	if err := mqttCli.Connect(); err != nil {
		log.Printf("MQTT connect warning (continuing without MQTT): %v", err)
	}
	defer mqttCli.Disconnect()

	h := handlers.NewHandler(cfg, mqttCli)
	rootCtx := context.Background()
	h.LoadSiteNames(rootCtx)

	go startScheduledTasks(cfg, h)
	go startAlarmProcessor(mqttCli, h)

	r := setupRouter(h)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("Server starting on :%d (mode=%s)", cfg.Server.Port, cfg.Server.Mode)
		log.Printf("API endpoints:")
		log.Printf("  GET    /api/sites                  栈道遗址列表")
		log.Printf("  GET    /api/sites/:id              单遗址详情")
		log.Printf("  POST   /api/sensor                 传感器数据上报")
		log.Printf("  POST   /api/sensor/batch           批量传感器数据上报")
		log.Printf("  GET    /api/sensor                 查询传感器数据")
		log.Printf("  GET    /api/sites/:id/daily        每日汇总")
		log.Printf("  POST   /api/sites/:id/simulate     结构仿真(有限元)")
		log.Printf("  GET    /api/sites/:id/simulation   最新仿真结果")
		log.Printf("  POST   /api/sites/:id/weathering   风化评估")
		log.Printf("  GET    /api/sites/:id/weathering   最新风化评估")
		log.Printf("  GET    /api/alarms                 告警列表")
		log.Printf("  GET    /api/dashboard              仪表盘数据")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func setupRouter(h *handlers.Handler) *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.Static("/static", "./static")
	r.StaticFile("/", "./static/index.html")

	api := r.Group("/api")
	{
		api.GET("/sites", h.GetSites)
		api.GET("/sites/:id", h.GetSite)
		api.GET("/sites/:id/daily", h.GetDailySummary)
		api.GET("/sites/:id/simulation", h.GetLatestSimulation)
		api.GET("/sites/:id/weathering", h.GetLatestWeathering)
		api.GET("/sites/:id/thresholds", h.GetThresholds)

		api.POST("/sensor", h.PostSensorReading)
		api.POST("/sensor/batch", h.PostBatchSensorReadings)
		api.GET("/sensor", h.GetSensorReadings)

		api.POST("/sites/:id/simulate", h.RunSimulation)
		api.POST("/sites/:id/weathering", h.RunWeathering)

		api.GET("/alarms", h.GetAlarms)
		api.POST("/alarms/:id/ack", h.AckAlarm)
		api.POST("/alarms/:id/resolve", h.ResolveAlarm)

		api.GET("/dashboard", h.GetDashboard)
	}

	return r
}

func startScheduledTasks(cfg *config.Config, h *handlers.Handler) {
	simTicker := time.NewTicker(1 * time.Hour)
	weatherTicker := time.NewTicker(6 * time.Hour)
	defer simTicker.Stop()
	defer weatherTicker.Stop()

	siteRepo := repository.NewSiteRepo()
	sensorRepo := repository.NewSensorRepo()
	simRepo := repository.NewSimulationRepo()
	weatherRepo := repository.NewWeatheringRepo()

	femSolver := simulation.NewSolver(&cfg.FEM)
	assessor := weathering.NewAssessor(&cfg.Weather)

	mqttClient := h.MQTTClient()

	for {
		select {
		case <-simTicker.C:
			runStructuralSimulations(siteRepo, sensorRepo, simRepo, femSolver, mqttClient)
		case <-weatherTicker.C:
			runWeatheringAssessments(siteRepo, sensorRepo, simRepo, weatherRepo, assessor, mqttClient)
		}
	}
}

func runStructuralSimulations(sr *repository.SiteRepo, sens *repository.SensorRepo,
	simRepo *repository.SimulationRepo, solver *simulation.Solver, mqtt *mqttclient.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	sites, err := sr.GetAll(ctx)
	if err != nil {
		log.Printf("[FEM] get sites error: %v", err)
		return
	}

	for _, site := range sites {
		go func(s models.PlankroadSite) {
			readings, _ := sens.GetBySite(ctx, s.SiteID, time.Now().Add(-48*time.Hour), time.Now(), 500)
			sim, err := solver.Simulate(&s, readings)
			if err != nil {
				log.Printf("[FEM] site %d error: %v", s.SiteID, err)
				return
			}
			if err := simRepo.Save(ctx, sim); err != nil {
				log.Printf("[FEM] save site %d error: %v", s.SiteID, err)
				return
			}
			mqtt.PublishSimulation(s.SiteID, sim)
		}(site)
	}
}

func runWeatheringAssessments(sr *repository.SiteRepo, sens *repository.SensorRepo,
	simRepo *repository.SimulationRepo, wr *repository.WeatheringRepo,
	assessor *weathering.Assessor, mqtt *mqttclient.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	sites, err := sr.GetAll(ctx)
	if err != nil {
		log.Printf("[Weathering] get sites error: %v", err)
		return
	}

	for _, site := range sites {
		go func(s models.PlankroadSite) {
			readings, _ := sens.GetBySite(ctx, s.SiteID, time.Now().Add(-720*time.Hour), time.Now(), 2000)
			sim, _ := simRepo.GetLatest(ctx, s.SiteID)
			assess := assessor.Assess(&s, readings, sim)
			if err := wr.Save(ctx, assess); err != nil {
				log.Printf("[Weathering] save site %d error: %v", s.SiteID, err)
				return
			}
			mqtt.PublishWeathering(s.SiteID, assess)
		}(site)
	}
}

func startAlarmProcessor(mqttCli *mqttclient.Client, h *handlers.Handler) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		published, err := mqttCli.ProcessPendingAlarms(ctx, h.SiteNames())
		cancel()
		if err != nil {
			continue
		}
		_ = published
	}
}
