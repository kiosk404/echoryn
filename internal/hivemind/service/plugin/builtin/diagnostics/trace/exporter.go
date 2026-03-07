package trace

import (
	"context"
	"sync"
)

// Exporter is the interface for trace data export backends.
//
// Exporters receive completed spans and are responsible for serializing,
// batching, and transmitting them to their destination (OTLP endpoint,
// in-memory store, log file, etc.).
type Exporter interface {
	// ExportSpans receives a batch of completed spans for export.
	// Implementations should be safe for concurrent use.
	ExportSpans(ctx context.Context, spans []*Span) error

	// Shutdown gracefully shuts down the exporter, flushing any pending data.
	Shutdown(ctx context.Context) error
}

// MemoryExporter stores spans in-memory for inspection and testing.
//
// This is the default exporter when OTEL export is not configured,
// providing in-process access to trace data for the diagnostics_status tool.
type MemoryExporter struct {
	mu     sync.RWMutex
	traces map[TraceID]*Trace
	spans  []*Span
	cap    int
}

// NewMemoryExporter creates a MemoryExporter with the given capacity.
// When capacity is exceeded, oldest spans are discarded (ring buffer).
func NewMemoryExporter(capacity int) *MemoryExporter {
	if capacity <= 0 {
		capacity = 1000
	}
	return &MemoryExporter{
		traces: make(map[TraceID]*Trace),
		spans:  make([]*Span, 0, capacity),
		cap:    capacity,
	}
}

// ExportSpans stores spans in the in-memory buffer.
func (e *MemoryExporter) ExportSpans(_ context.Context, spans []*Span) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, span := range spans {
		// Add to global span list with ring buffer eviction.
		if len(e.spans) >= e.cap {
			// Evict oldest spans and their trace references.
			evicted := e.spans[0]
			e.spans = e.spans[1:]
			e.cleanupTrace(evicted.TraceID, evicted.SpanID)
		}
		e.spans = append(e.spans, span)

		// Add to trace index.
		tr, ok := e.traces[span.TraceID]
		if !ok {
			tr = &Trace{
				TraceID:   span.TraceID,
				Spans:     make(map[SpanID]*Span),
				StartTime: span.StartTime,
			}
			e.traces[span.TraceID] = tr
		}
		tr.Spans[span.SpanID] = span

		// Update trace root and timing.
		if span.IsRoot() {
			tr.RootSpanID = span.SpanID
		}
		if span.StartTime.Before(tr.StartTime) {
			tr.StartTime = span.StartTime
		}
		if !span.EndTime.IsZero() && span.EndTime.After(tr.EndTime) {
			tr.EndTime = span.EndTime
		}
	}

	return nil
}

// Shutdown is a no-op for the memory exporter.
func (e *MemoryExporter) Shutdown(_ context.Context) error {
	return nil
}

// GetTrace returns a trace by its ID, or nil if not found.
func (e *MemoryExporter) GetTrace(traceID TraceID) *Trace {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.traces[traceID]
}

// ListTraces returns all stored traces, ordered by start time (newest first).
func (e *MemoryExporter) ListTraces(limit int) []*Trace {
	e.mu.RLock()
	defer e.mu.RUnlock()

	traces := make([]*Trace, 0, len(e.traces))
	for _, t := range e.traces {
		traces = append(traces, t)
	}

	// Sort by start time descending (newest first).
	for i := 0; i < len(traces); i++ {
		for j := i + 1; j < len(traces); j++ {
			if traces[j].StartTime.After(traces[i].StartTime) {
				traces[i], traces[j] = traces[j], traces[i]
			}
		}
	}

	if limit > 0 && len(traces) > limit {
		traces = traces[:limit]
	}
	return traces
}

// SpanCount returns the total number of stored spans.
func (e *MemoryExporter) SpanCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.spans)
}

// TraceCount returns the total number of stored traces.
func (e *MemoryExporter) TraceCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.traces)
}

// RecentSpans returns the most recent N spans.
func (e *MemoryExporter) RecentSpans(n int) []*Span {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if n <= 0 || len(e.spans) == 0 {
		return nil
	}
	start := len(e.spans) - n
	if start < 0 {
		start = 0
	}

	result := make([]*Span, len(e.spans)-start)
	copy(result, e.spans[start:])

	// Reverse to return newest first.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// cleanupTrace removes a span from its trace, and the trace itself if empty.
func (e *MemoryExporter) cleanupTrace(traceID TraceID, spanID SpanID) {
	tr, ok := e.traces[traceID]
	if !ok {
		return
	}
	delete(tr.Spans, spanID)
	if len(tr.Spans) == 0 {
		delete(e.traces, traceID)
	}
}

// MultiExporter fans out span exports to multiple backends.
type MultiExporter struct {
	exporters []Exporter
}

// NewMultiExporter creates an exporter that writes to all given backends.
func NewMultiExporter(exporters ...Exporter) *MultiExporter {
	return &MultiExporter{exporters: exporters}
}

// ExportSpans sends spans to all underlying exporters.
// Returns the first error encountered (but continues exporting to remaining).
func (m *MultiExporter) ExportSpans(ctx context.Context, spans []*Span) error {
	var firstErr error
	for _, exp := range m.exporters {
		if err := exp.ExportSpans(ctx, spans); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Shutdown shuts down all underlying exporters.
func (m *MultiExporter) Shutdown(ctx context.Context) error {
	var firstErr error
	for _, exp := range m.exporters {
		if err := exp.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Compile-time interface checks.
var (
	_ Exporter = (*MemoryExporter)(nil)
	_ Exporter = (*MultiExporter)(nil)
)
