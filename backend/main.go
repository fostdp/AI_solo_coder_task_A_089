package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"plankroad-backend/config"
	"plankroad-backend/config/params"
	"plankroad-backend/database"
	"plankroad-backend/handlers"
	"plankroad-backend/modules/alarm_mqtt"
	"plankroad-backend/modules/bus"
	"plankroad-backend/modules/dtu_receiver"
	"plankroad-backend/modules/structural_simulator"
	"plankroad-backend/modules/weathering_evaluator"
)

type App struct {
	cfg             *config.Config
	logger          *log.Logger
	messageBus      *bus.Bus
	dtuReceiver     *dtu_receiver.DTUReceiver
	structuralSim   *structural_simulator.StructuralSimulator
	weatheringEval  *weathering_evaluator.WeatheringEvaluator
	alarmMQTT       *alarm_mqtt.AlarmMQTT
	handler         *handlers.Handler
	ginEngine       *gin.Engine
	wg              sync.WaitGroup
	shutdownTimeout time.Duration
}

func main() {
	app := &App{
		logger:          log.New(os.Stdout, "[PLANKROAD] ", log.LstdFlags|log.Lmicroseconds),
		shutdownTimeout: 30 * time.Second,
	}

	if err := app.init(); err != nil {
		app.logger.Fatalf("Initialization failed: %v", err)
	}
	defer app.cleanup()

	app.startScheduledTasks()
	app.startHTTPServer()
	app.waitForShutdown()
}

func (a *App) init() error {
	a.logger.Println("========================================")
	a.logger.Println("  秦巴山区古栈道结构力学仿真与风化评估系统")
	a.logger.Println("  模块化架构启动")
	a.logger.Println("========================================")

	if err := godotenv.Load(); err != nil {
		a.logger.Printf("Warning: .env file not found, using defaults: %v", err)
	}

	cfg := config.Load()
	a.cfg = cfg

	workDir, _ := os.Getwd()
	a.logger.Printf("Working directory: %s", workDir)

	paramsPath := filepath.Join(workDir, "config", "params")
	if err := params.LoadAll(paramsPath); err != nil {
		return a.fail("load params JSON", err)
	}
	a.logger.Printf("Loaded %d rock types + %d wood types from JSON config",
		len(params.LoadedRockParams), len(params.LoadedWoodParams))

	if err := database.Init(&cfg.Database); err != nil {
		return a.fail("init database", err)
	}
	a.logger.Printf("Database connected: %s:%d/%s", cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	a.messageBus = bus.New(a.logger)
	a.logger.Println("Message bus initialized")

	a.logger.Println("--- Initializing modules ---")

	var err error
	a.dtuReceiver, err = dtu_receiver.New(cfg, a.messageBus, a.logger)
	if err != nil {
		return a.fail("init DTU receiver", err)
	}
	a.logger.Println("✓ DTU Receiver (data acquisition + validation) initialized")

	a.alarmMQTT, err = alarm_mqtt.New(cfg, a.messageBus, a.logger)
	if err != nil {
		a.logger.Printf("Warning: alarm MQTT init failed: %v", err)
	}
	a.logger.Println("✓ Alarm MQTT (alarm evaluation + push) initialized")

	a.structuralSim, err = structural_simulator.New(cfg, a.messageBus, a.logger, a.alarmMQTT)
	if err != nil {
		return a.fail("init structural simulator", err)
	}
	a.logger.Println("✓ Structural Simulator (FEM + contact algorithm) initialized")

	a.weatheringEval, err = weathering_evaluator.New(cfg, a.messageBus, a.logger, a.alarmMQTT)
	if err != nil {
		return a.fail("init weathering evaluator", err)
	}
	a.logger.Println("✓ Weathering Evaluator (freeze-thaw + crack propagation) initialized")

	a.handler = handlers.New(cfg, a.messageBus, a.dtuReceiver, a.structuralSim, a.weatheringEval, a.alarmMQTT)
	a.logger.Println("✓ API Handler initialized")

	a.setupGin()
	a.logger.Println("✓ Gin router initialized")

	a.alarmMQTT.StartAlarmProcessor(time.Duration(cfg.Alarm.PendingCheckSec) * time.Second)
	a.logger.Println("✓ Alarm processor started")

	a.logger.Println("--- All modules initialized successfully ---")
	a.logger.Printf("Go version: %s | GOMAXPROCS: %d", runtime.Version(), runtime.GOMAXPROCS(0))
	return nil
}

func (a *App) setupGin() {
	gin.SetMode(gin.ReleaseMode)
	a.ginEngine = gin.New()
	a.ginEngine.Use(gin.Recovery())
	a.ginEngine.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return a.logger.Prefix() + "HTTP " + param.Method + " " + param.Path +
			" " + fmt.Sprintf("%d", param.StatusCode) + " " + param.Latency.String() + "\n"
	}))

	a.ginEngine.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	a.ginEngine.Static("/static", "./static")
	a.ginEngine.StaticFile("/", "./static/index.html")
	a.ginEngine.StaticFile("/texture_params.json", "./config/params/texture_params.json")

	api := a.ginEngine.Group("/api")
	{
		api.GET("/sites", a.handler.GetSites)
		api.GET("/sites/:id", a.handler.GetSite)

		api.POST("/sensor/batch", a.handler.PostBatchSensorReadings)
		api.POST("/sensor/single", a.handler.PostSingleSensorReading)
		api.GET("/sites/:id/readings", a.handler.GetRecentReadings)

		api.POST("/sites/:id/simulate", a.handler.RunSimulation)
		api.POST("/simulate/all", a.handler.TriggerSimAll)
		api.GET("/sites/:id/simulation", a.handler.GetLatestSimulation)

		api.POST("/sites/:id/weathering", a.handler.RunWeathering)
		api.POST("/weathering/all", a.handler.TriggerWeatheringAll)
		api.GET("/sites/:id/weathering", a.handler.GetLatestWeathering)

		api.GET("/alarms", a.handler.GetAlarms)
		api.POST("/alarms/:id/ack", a.handler.AckAlarm)
		api.POST("/alarms/:id/resolve", a.handler.ResolveAlarm)

		api.GET("/dashboard", a.handler.GetDashboard)
		api.GET("/sites/:id/summary", a.handler.GetDailySummary)
	}
}

func (a *App) startScheduledTasks() {
	a.logger.Println("Starting scheduled tasks...")

	simTicker := time.NewTicker(time.Duration(a.cfg.FEM.RunIntervalHr) * time.Hour)
	weatherTicker := time.NewTicker(time.Duration(a.cfg.Weather.RunIntervalHr) * time.Hour)

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.logger.Printf("Structural simulation scheduled every %dh", a.cfg.FEM.RunIntervalHr)
		for range simTicker.C {
			a.logger.Println("[TIMER] Running structural simulations for all sites...")
			if err := a.structuralSim.RunAll("timer"); err != nil {
				a.logger.Printf("[TIMER] Simulations failed: %v", err)
			}
		}
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.logger.Printf("Weathering assessment scheduled every %dh", a.cfg.Weather.RunIntervalHr)
		for range weatherTicker.C {
			a.logger.Println("[TIMER] Running weathering assessments for all sites...")
			if err := a.weatheringEval.RunAll("timer"); err != nil {
				a.logger.Printf("[TIMER] Assessments failed: %v", err)
			}
		}
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		healthTicker := time.NewTicker(60 * time.Second)
		for range healthTicker.C {
			metrics := a.messageBus.GetMetrics()
			a.logger.Printf("[HEALTH] Bus events: %+v", metrics)
		}
	}()

	go func() {
		time.Sleep(5 * time.Second)
		a.logger.Println("[INIT] Running initial structural simulation...")
		if _, err := a.structuralSim.RunForSite(1, "init"); err != nil {
			a.logger.Printf("[INIT] Initial sim failed: %v", err)
		}

		time.Sleep(3 * time.Second)
		a.logger.Println("[INIT] Running initial weathering assessment...")
		if _, err := a.weatheringEval.RunForSite(1, "init"); err != nil {
			a.logger.Printf("[INIT] Initial weathering failed: %v", err)
		}
	}()
}

func (a *App) startHTTPServer() {
	addr := ":" + fmt.Sprintf("%d", a.cfg.Server.Port)
	a.logger.Printf("HTTP server starting on %s", addr)

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.ginEngine.Run(addr); err != nil {
			a.logger.Printf("HTTP server stopped: %v", err)
		}
	}()
}

func (a *App) waitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	a.logger.Printf("Received signal: %v, initiating graceful shutdown...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		a.cleanup()
		close(done)
	}()

	select {
	case <-done:
		a.logger.Println("Graceful shutdown completed")
	case <-ctx.Done():
		a.logger.Printf("Shutdown timed out after %v, force exiting", a.shutdownTimeout)
		os.Exit(1)
	}
}

func (a *App) cleanup() {
	a.logger.Println("Cleaning up resources...")

	if a.dtuReceiver != nil {
		a.dtuReceiver.Close()
		a.logger.Println("  ✓ DTU Receiver closed")
	}

	if a.alarmMQTT != nil {
		a.alarmMQTT.Close()
		a.logger.Println("  ✓ Alarm MQTT closed")
	}

	if a.messageBus != nil {
		a.messageBus.Close()
		a.logger.Println("  ✓ Message bus closed")
	}

	if database.Pool != nil {
		database.Pool.Close()
		a.logger.Println("  ✓ Database connection closed")
	}

	a.logger.Println("All resources released")
}

func (a *App) fail(step string, err error) error {
	a.logger.Fatalf("Failed at step [%s]: %v", step, err)
	return err
}
