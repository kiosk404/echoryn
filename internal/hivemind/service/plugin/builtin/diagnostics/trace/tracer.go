package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// TracerConfig holds configuration for the Tracer.
type TracerConfig struct {
	// ServiceName is the OTEL service.name resource attribute.
	ServiceName string

	// ServiceVersion is the service version.
	ServiceVersion string

	// SampleRate controls trace sampling (0.0 to 1.0, default 1.0).
	SampleRate float64

	// BatchSize is the number of spans to accumulate before flushing to the exporter.
	// Default: 64.
	BatchSize int

	// FlushInterval is the maximum time between automatic flushes.
	// Default: 5s.
	FlushInterval time.Duration

	// Exporter is the destination for completed spans.
	Exporter Exporter
}

// DefaultTracerConfig returns sensible defaults.
func DefaultTracerConfig() TracerConfig {
	return TracerConfig{
		ServiceName:    "echoryn-hivemind",
		ServiceVersion: "dev",
		SampleRate:     1.0,
		BatchSize:      64,
		FlushInterval:  5 * time.Second,
	}
}

// Tracer manages the lifecycle of spans and exports them to configured backends.
//
// Modeled after OpenClaw's diagnostics-otel tracer with these additions:
//   - LLM-specific SpanKinds and semantic attributes
//   - Built-in sampling (probability-based)
//   - Asynchronous batch export with periodic flush
//   - Context propagation via Go context
//
// Usage:
//
//	ctx, span := tracer.Start(ctx, "llm.call/openai/gpt-4o", trace.SpanKindLLMCall)
//	defer span.End(trace.SpanStatusOK, "")
//	span.SetAttribute(trace.AttrGenAIRequestModel, "gpt-4o")
type Tracer struct {
	cfg      TracerConfig
	resource *Resource
	sampler  Sampler
	exporter Exporter

	// pending holds spans waiting to be flushed.
	mu      sync.Mutex
	pending []*Span

	// done signals the flush goroutine to stop.
	done     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// NewTracer creates a new Tracer with the given configuration.
func NewTracer(cfg TracerConfig) *Tracer {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 64
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}
	if cfg.Exporter == nil {
		cfg.Exporter = NewMemoryExporter(1000)
	}

	t := &Tracer{
		cfg: cfg,
		resource: &Resource{
			ServiceName:    cfg.ServiceName,
			ServiceVersion: cfg.ServiceVersion,
		},
		sampler:  NewSampler(cfg.SampleRate),
		exporter: cfg.Exporter,
		pending:  make([]*Span, 0, cfg.BatchSize),
		done:     make(chan struct{}),
	}

	// Start the background flush goroutine.
	t.wg.Add(1)
	go t.flushLoop()

	return t
}

// Start begins a new span as a child of the active span in ctx.
//
// If no parent span exists in ctx, a new trace is created (root span).
// The returned context carries the new span; pass it to downstream calls
// for automatic parent-child linking.
//
// If the sampler rejects this trace, a no-op span is returned that
// does not record attributes or export.
func (t *Tracer) Start(ctx context.Context, name string, kind SpanKind) (context.Context, *Span) {
	var traceID TraceID
	var parentID SpanID

	// Inherit trace from parent span if present.
	if parent := SpanFromContext(ctx); parent != nil {
		traceID = parent.TraceID
		parentID = parent.SpanID
	} else {
		traceID = generateTraceID()
	}

	// Check sampling decision.
	if !t.sampler.ShouldSample(traceID, name, kind) {
		// Return a no-op span that won't be exported.
		noopSpan := &Span{
			TraceID:      traceID,
			SpanID:       generateSpanID(),
			ParentSpanID: parentID,
			Name:         name,
			Kind:         kind,
			StartTime:    time.Now(),
		}
		return ContextWithSpan(ctx, noopSpan), noopSpan
	}

	span := &Span{
		TraceID:      traceID,
		SpanID:       generateSpanID(),
		ParentSpanID: parentID,
		Name:         name,
		Kind:         kind,
		StartTime:    time.Now(),
		Attributes:   make(map[string]interface{}),
	}

	return ContextWithSpan(ctx, span), span
}

// EndSpan ends the given span and enqueues it for export.
//
// This is a convenience method that combines Span.End() with export enqueue.
// Prefer this over calling span.End() directly to ensure spans are exported.
func (t *Tracer) EndSpan(span *Span, status SpanStatus, statusMessage string) {
	if span == nil {
		return
	}
	span.End(status, statusMessage)
	t.enqueue(span)
}

// enqueue adds a completed span to the pending buffer.
// Triggers a flush if the batch size is reached.
func (t *Tracer) enqueue(span *Span) {
	t.mu.Lock()
	t.pending = append(t.pending, span)
	shouldFlush := len(t.pending) >= t.cfg.BatchSize
	t.mu.Unlock()

	if shouldFlush {
		t.flush()
	}
}

// flush exports all pending spans to the exporter.
func (t *Tracer) flush() {
	t.mu.Lock()
	if len(t.pending) == 0 {
		t.mu.Unlock()
		return
	}
	batch := t.pending
	t.pending = make([]*Span, 0, t.cfg.BatchSize)
	t.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := t.exporter.ExportSpans(ctx, batch); err != nil {
		logger.Warn("[Trace] failed to export %d spans: %v", len(batch), err)
	}
}

// flushLoop periodically flushes pending spans.
func (t *Tracer) flushLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.flush()
		case <-t.done:
			// Final flush before exit.
			t.flush()
			return
		}
	}
}

// Shutdown stops the flush goroutine and shuts down the exporter.
// Should be called during graceful shutdown.
func (t *Tracer) Shutdown(ctx context.Context) error {
	var err error
	t.stopOnce.Do(func() {
		close(t.done)
		t.wg.Wait()

		// Final flush.
		t.flush()
	
		err = t.exporter.Shutdown(ctx)
	})

	return err
}

// Resource returns the tracer's resource descriptor.
func (t *Tracer) Resource() *Resource {
	return t.resource
}

// generateTraceID generates a random 16-byte trace ID.
func generateTraceID() TraceID {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return TraceID(hex.EncodeToString(b))
}

// generateSpanID generates a random 8-byte span ID.
func generateSpanID() SpanID {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return SpanID(hex.EncodeToString(b))
}
