package bus

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	"plankroad-backend/models"
)

type EventType string

const (
	EventSensorDataReceived   EventType = "sensor.data.received"
	EventSensorDataValidated  EventType = "sensor.data.validated"
	EventStructuralSimReady  EventType = "structural.simulation.ready"
	EventWeatheringReady     EventType = "weathering.assessment.ready"
	EventAlarmRaised         EventType = "alarm.raised"
	EventAlarmPublished      EventType = "alarm.published"
	EventCommandSimulate     EventType = "command.simulate"
	EventCommandWeathering   EventType = "command.weathering"
)

type Event struct {
	Type      EventType
	Timestamp time.Time
	SiteID    int
	Payload   interface{}
	RequestID string
}

type EventHandler func(ctx context.Context, evt Event)

type Bus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]handlerEntry
	logger      *log.Logger
	metrics     map[EventType]uint64
}

type handlerEntry struct {
	id       string
	handler  EventHandler
	bufSize  int
	ch       chan Event
	ctx      context.Context
	cancel   context.CancelFunc
}

func New(logger *log.Logger) *Bus {
	if logger == nil {
		logger = log.Default()
	}
	return &Bus{
		subscribers: make(map[EventType][]handlerEntry),
		logger:      logger,
		metrics:     make(map[EventType]uint64),
	}
}

func (b *Bus) Subscribe(ctx context.Context, eventType EventType, handlerID string,
	bufSize int, handler EventHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if bufSize <= 0 {
		bufSize = 100
	}

	handlerCtx, cancel := context.WithCancel(ctx)
	ch := make(chan Event, bufSize)

	entry := handlerEntry{
		id:      handlerID,
		handler: handler,
		bufSize: bufSize,
		ch:      ch,
		ctx:     handlerCtx,
		cancel:  cancel,
	}

	go b.dispatchLoop(entry)

	b.subscribers[eventType] = append(b.subscribers[eventType], entry)
	b.logger.Printf("[BUS] Subscribed %s to %s (buf=%d)", handlerID, eventType, bufSize)
	return nil
}

func (b *Bus) dispatchLoop(entry handlerEntry) {
	for {
		select {
		case <-entry.ctx.Done():
			b.logger.Printf("[BUS] Dispatch loop %s stopped", entry.id)
			return
		case evt := <-entry.ch:
			func() {
				defer func() {
					if r := recover(); r != nil {
						b.logger.Printf("[BUS] Panic in handler %s for %s: %v",
							entry.id, evt.Type, r)
					}
				}()
				entry.handler(entry.ctx, evt)
			}()
		}
	}
}

func (b *Bus) Publish(ctx context.Context, evt Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	handlers, ok := b.subscribers[evt.Type]
	if !ok || len(handlers) == 0 {
		b.logger.Printf("[BUS] No subscribers for %s (site=%d)", evt.Type, evt.SiteID)
		return nil
	}

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}

	b.metrics[evt.Type]++

	var errs []error
	for _, h := range handlers {
		select {
		case h.ch <- evt:
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("handler %s: %w", h.id, ctx.Err()))
		default:
			errs = append(errs, fmt.Errorf("handler %s buffer full for %s", h.id, evt.Type))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("publish errors: %v", errs)
	}
	return nil
}

func (b *Bus) Request(ctx context.Context, reqType, respType EventType,
	siteID int, payload interface{}, timeout time.Duration) (interface{}, error) {
	reqID := fmt.Sprintf("req-%d-%s", siteID, time.Now().Format("20060102150405.000"))

	respCh := make(chan interface{}, 1)
	once := sync.Once{}

	subID := fmt.Sprintf("rpc-%s", reqID)
	b.Subscribe(ctx, respType, subID, 1, func(c context.Context, e Event) {
		if e.RequestID == reqID {
			once.Do(func() { respCh <- e.Payload })
		}
	})
	defer b.Unsubscribe(respType, subID)

	reqEvt := Event{
		Type:      reqType,
		SiteID:    siteID,
		Payload:   payload,
		RequestID: reqID,
	}

	if err := b.Publish(ctx, reqEvt); err != nil {
		return nil, fmt.Errorf("request publish: %w", err)
	}

	select {
	case resp := <-respCh:
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("request %s timed out after %v", reqType, timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *Bus) Unsubscribe(eventType EventType, handlerID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers, ok := b.subscribers[eventType]
	if !ok {
		return
	}

	for i, h := range handlers {
		if h.id == handlerID {
			h.cancel()
			close(h.ch)
			b.subscribers[eventType] = append(handlers[:i], handlers[i+1:]...)
			b.logger.Printf("[BUS] Unsubscribed %s from %s", handlerID, eventType)
			return
		}
	}
}

func (b *Bus) GetMetrics() map[string]uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	m := make(map[string]uint64, len(b.metrics))
	for k, v := range b.metrics {
		m[string(k)] = v
	}
	return m
}

func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for evtType, handlers := range b.subscribers {
		for _, h := range handlers {
			h.cancel()
			close(h.ch)
		}
		b.logger.Printf("[BUS] Closed %d handlers for %s", len(handlers), evtType)
		delete(b.subscribers, evtType)
	}
	b.logger.Println("[BUS] Bus closed")
}

type SensorDataPayload struct {
	Reading  *models.SensorReading
	IsBatch  bool
	Batch    []models.SensorReading
	Source   string
}

type SimulationPayload struct {
	Simulation *models.StructuralSimulation
	Readings   []models.SensorReading
}

type WeatheringPayload struct {
	Assessment *models.WeatheringAssessment
	Readings   []models.SensorReading
	Sim        *models.StructuralSimulation
}

type AlarmPayload struct {
	Alarm    *models.AlarmEvent
	SiteName string
}

func (e Event) PayloadAsSensorData() (*SensorDataPayload, bool) {
	p, ok := e.Payload.(*SensorDataPayload)
	return p, ok
}

func (e Event) PayloadAsSimulation() (*SimulationPayload, bool) {
	p, ok := e.Payload.(*SimulationPayload)
	return p, ok
}

func (e Event) PayloadAsWeathering() (*WeatheringPayload, bool) {
	p, ok := e.Payload.(*WeatheringPayload)
	return p, ok
}

func (e Event) PayloadAsAlarm() (*AlarmPayload, bool) {
	p, ok := e.Payload.(*AlarmPayload)
	return p, ok
}

func EventTypeOf(v interface{}) EventType {
	return EventType(reflect.TypeOf(v).String())
}
