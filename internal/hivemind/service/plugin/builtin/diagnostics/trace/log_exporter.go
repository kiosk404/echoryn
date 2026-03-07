package trace

import (
	"context"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// LogExporter exports spans to the application logger.
//
// This exporter is useful for development and debugging - it logs
// each completed span as a structured log entry.
type LogExporter struct {
	// Verbose controls whether full attributes are logged.
	// When false, only span name, kind, duration, and status are logged.
	Verbose bool
}

// NewLogExporter creates a LogExporter.
func NewLogExporter(verbose bool) *LogExporter {
	return &LogExporter{Verbose: verbose}
}

// ExportSpans logs each span to the application logger.
func (e *LogExporter) ExportSpans(_ context.Context, spans []*Span) error {
	for _, span := range spans {
		if e.Verbose {
			logger.Debug("[Trace] span=%s kind=%s name=%s status=%s duration=%s attr=%v",
				span.SpanID, span.Kind, span.Name, span.Status, span.Duration, span.Attributes)
		} else {
			logger.Debug("[Trace] span=%s kind=%s name=%q status=%s duration=%s",
				span.SpanID, span.Kind, span.Name, span.Status, span.Duration)
		}
	}
	return nil
}

// Shutdown is a no-op for the log exporter.
func (e *LogExporter) Shutdown(_ context.Context) error {
	return nil
}

// Compile-time interface check.
var _ Exporter = (*LogExporter)(nil)
