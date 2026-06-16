package monitoring

import (
	"context"
	"log"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry     *prometheus.Registry
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	activeConns  prometheus.Gauge

	sensorReadings   prometheus.Counter
	femSimulations   prometheus.Counter
	weatheringEvals  prometheus.Counter
	alarmsGenerated  prometheus.Counter
	alarmsPublished  prometheus.Counter
	mqttMessagesSent prometheus.Counter

	busEventsPublished *prometheus.CounterVec
	busEventsHandled   *prometheus.CounterVec

	mu     sync.RWMutex
	server *http.Server
	logger *log.Logger
}

func NewMetrics(logger *log.Logger) *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		logger:   logger,
	}

	m.httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "plankroad_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	m.httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "plankroad_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	m.activeConns = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "plankroad_http_active_connections",
			Help: "Number of active HTTP connections",
		},
	)

	m.sensorReadings = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "plankroad_sensor_readings_total",
			Help: "Total number of sensor readings received",
		},
	)

	m.femSimulations = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "plankroad_fem_simulations_total",
			Help: "Total number of FEM simulations completed",
		},
	)

	m.weatheringEvals = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "plankroad_weathering_evaluations_total",
			Help: "Total number of weathering evaluations completed",
		},
	)

	m.alarmsGenerated = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "plankroad_alarms_generated_total",
			Help: "Total number of alarms generated",
		},
	)

	m.alarmsPublished = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "plankroad_alarms_published_total",
			Help: "Total number of alarms published via MQTT",
		},
	)

	m.mqttMessagesSent = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "plankroad_mqtt_messages_sent_total",
			Help: "Total number of MQTT messages sent",
		},
	)

	m.busEventsPublished = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "plankroad_bus_events_published_total",
			Help: "Total number of events published to bus",
		},
		[]string{"event_type"},
	)

	m.busEventsHandled = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "plankroad_bus_events_handled_total",
			Help: "Total number of events handled from bus",
		},
		[]string{"event_type", "handler"},
	)

	m.registry.MustRegister(
		m.httpRequests,
		m.httpDuration,
		m.activeConns,
		m.sensorReadings,
		m.femSimulations,
		m.weatheringEvals,
		m.alarmsGenerated,
		m.alarmsPublished,
		m.mqttMessagesSent,
		m.busEventsPublished,
		m.busEventsHandled,
	)

	return m
}

func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

func (m *Metrics) IncSensorReadings() {
	m.sensorReadings.Inc()
}

func (m *Metrics) IncFEMSimulations() {
	m.femSimulations.Inc()
}

func (m *Metrics) IncWeatheringEvals() {
	m.weatheringEvals.Inc()
}

func (m *Metrics) IncAlarmsGenerated() {
	m.alarmsGenerated.Inc()
}

func (m *Metrics) IncAlarmsPublished() {
	m.alarmsPublished.Inc()
}

func (m *Metrics) IncMQTTMessagesSent() {
	m.mqttMessagesSent.Inc()
}

func (m *Metrics) IncBusEventsPublished(eventType string) {
	m.busEventsPublished.WithLabelValues(eventType).Inc()
}

func (m *Metrics) IncBusEventsHandled(eventType, handler string) {
	m.busEventsHandled.WithLabelValues(eventType, handler).Inc()
}

func (m *Metrics) ObserveHTTPRequest(method, path string, status int, duration time.Duration) {
	m.httpRequests.WithLabelValues(method, path, http.StatusText(status)).Inc()
	m.httpDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

func (m *Metrics) SetActiveConns(n float64) {
	m.activeConns.Set(n)
}

func (m *Metrics) StartPProf(port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	m.mu.Lock()
	m.server = server
	m.mu.Unlock()

	m.logger.Printf("PProf server starting on port %s (http://localhost:%s/debug/pprof/)", port, port)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Printf("PProf server error: %v", err)
		}
	}()
	return nil
}

func (m *Metrics) StartMetrics(port string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		m.logger.Printf("Prometheus metrics starting on port %s (http://localhost:%s/metrics)", port, port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Printf("Metrics server error: %v", err)
		}
	}()
	return nil
}

func (m *Metrics) Shutdown(ctx context.Context) error {
	m.mu.RLock()
	server := m.server
	m.mu.RUnlock()

	if server != nil {
		return server.Shutdown(ctx)
	}
	return nil
}
