package collector

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/diagnostics/entity"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// EventHandler is a callback for processing diagnostic events.
type EventHandler func(event *entity.DiagnosticEvent)

// Collector aggregates diagnostic metrics and dispatches events.
// It provides in-process metrics collection independent of any specific
// export backend (OTEL, Prometheus, etc.).
type Collector struct {
	mu       sync.RWMutex
	handlers []EventHandler
	stopped  atomic.Bool

	// In-memory counters for quick access (no external dependency).
	counters sync.Map // map[string]*int64

	// Histograms store recent values for percentile calculation.
	histograms sync.Map // map[string]*HistogramBucket
}

// HistogramBucket is a simple ring buffer for histogram values.
type HistogramBucket struct {
	mu     sync.Mutex
	values []float64
	pos    int
	cap    int
}

// NewHistogramBucket creates a histogram bucket with the given capacity.
func NewHistogramBucket(capacity int) *HistogramBucket {
	return &HistogramBucket{
		values: make([]float64, 0, capacity),
		cap:    capacity,
	}
}

// Record adds a value to the histogram bucket.
func (h *HistogramBucket) Record(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.values) < h.cap {
		h.values = append(h.values, v)
	} else {
		h.values[h.pos] = v
	}
	h.pos = (h.pos + 1) % h.cap
}

// Values returns a copy of the recorded values.
func (h *HistogramBucket) Values() []float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]float64, len(h.values))
	copy(out, h.values)
	return out
}

// New creates a new diagnostic event Collector.
func New() *Collector {
	return &Collector{}
}

// Subscribe registers a handler that will receive diagnostic events.
func (c *Collector) Subscribe(handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = append(c.handlers, handler)
}

// Emit dispatches a diagnostic event to all subscribers.
func (c *Collector) Emit(event *entity.DiagnosticEvent) {
	if c.stopped.Load() {
		return
	}

	c.mu.RLock()
	handlers := make([]EventHandler, len(c.handlers))
	copy(handlers, c.handlers)
	c.mu.RUnlock()

	for _, h := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Warn("[Diagnostics] event handler panic: %v", r)
				}
			}()
			h(event)
		}()
	}
}

// IncrCounter increments a named counter by delta.
func (c *Collector) IncrCounter(name string, delta int64) {
	val, _ := c.counters.LoadOrStore(name, new(int64))
	atomic.AddInt64(val.(*int64), delta)
}

// GetCounter returns the current value of a named counter.
func (c *Collector) GetCounter(name string) int64 {
	val, ok := c.counters.Load(name)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(val.(*int64))
}

// RecordHistogram records a value in a named histogram.
func (c *Collector) RecordHistogram(name string, value float64) {
	bucket, _ := c.histograms.LoadOrStore(name, NewHistogramBucket(1000))
	bucket.(*HistogramBucket).Record(value)
}

// Stop prevents further event dispatching.
func (c *Collector) Stop() {
	c.stopped.Store(true)
}

// Snapshot returns a point-in-time snapshot of all counters.
func (c *Collector) Snapshot() map[string]int64 {
	result := make(map[string]int64)
	c.counters.Range(func(key, value interface{}) bool {
		result[key.(string)] = atomic.LoadInt64(value.(*int64))
		return true
	})
	return result
}

// EmitModelUsage is a convenience method for emitting model usage events.
func (c *Collector) EmitModelUsage(ctx context.Context, attrs *entity.ModelUsageAttrs) {
	c.IncrCounter("tokens.input", attrs.InputTokens)
	c.IncrCounter("tokens.output", attrs.OutputTokens)
	c.RecordHistogram("run.duration_ms", float64(attrs.DurationMs))

	c.Emit(&entity.DiagnosticEvent{
		Type: entity.EventModelUsage,
		Attrs: map[string]interface{}{
			"provider":      attrs.Provider,
			"model":         attrs.Model,
			"input_tokens":  attrs.InputTokens,
			"output_tokens": attrs.OutputTokens,
			"cost_usd":      attrs.CostUSD,
			"duration_ms":   attrs.DurationMs,
		},
	})
}

// EmitRunAttempt emits a run attempt event.
func (c *Collector) EmitRunAttempt(ctx context.Context, attrs *entity.RunAttemptAttrs) {
	c.IncrCounter("run.attempts", 1)
	if !attrs.Success {
		c.IncrCounter("run.errors", 1)
	}
	c.RecordHistogram("run.duration_ms", float64(attrs.DurationMs))

	a := map[string]interface{}{
		"provider":    attrs.Provider,
		"model":       attrs.Model,
		"duration_ms": attrs.DurationMs,
		"success":     attrs.Success,
	}
	if attrs.Error != "" {
		a["error"] = attrs.Error
	}

	c.Emit(&entity.DiagnosticEvent{
		Type:  entity.EventRunAttempt,
		Attrs: a,
	})
}

// StartTimer returns a function that, when called, records the elapsed
// duration in milliseconds to the named histogram.
func (c *Collector) StartTimer(histogramName string) func() time.Duration {
	start := time.Now()
	return func() time.Duration {
		d := time.Since(start)
		c.RecordHistogram(histogramName, float64(d.Milliseconds()))
		return d
	}
}
