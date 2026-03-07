package trace

import (
	"context"
)

// ctxKey is a private type for context keys to avoid collisions.
type ctxKey string

const (
	ctxKeyActiveSpan ctxKey = "echoryn.trace.active_span"
	ctxKeyTracer     ctxKey = "echoryn.trace.tracer"
)

// ContextWithSpan returns a new context carrying the give span.
// Downstream code cna retrieve the span via SpanFromContext.
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, ctxKeyActiveSpan, span)
}

// SpanFromContext extracts the active span from the context.
// Returns nil if no span is active.
func SpanFromContext(ctx context.Context) *Span {
	v, _ := ctx.Value(ctxKeyActiveSpan).(*Span)
	return v
}

// ContextWithTracer returns a new context carrying the given tracer.
// This allows code anywhere in the call chain to start new spans.
// via TracerFromContext
func ContextWithTracer(ctx context.Context, tracer *Tracer) context.Context {
	return context.WithValue(ctx, ctxKeyTracer, tracer)
}

// TracerFromContext extracts the tracer from the context.
// Returns nil if no tracer is set.
func TracerFromContext(ctx context.Context) *Tracer {
	v, _ := ctx.Value(ctxKeyTracer).(*Tracer)
	return v
}
