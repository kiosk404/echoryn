// Package trace provides LLM-aware distributed tracing for Echoryn.
//
// This package implements a lightweight tracing system aligned with
// OpenTelemetry's semantic conventions for LLM/GenAI workloads, while
// remaining independent of the OTEL SDK itself. Traces can be exported
// to OTLP-compatible backends or consumed in-process.
//
// Architecture (modeled after OpenClaw's diagnostics-otel trace layer):
//
//	Tracer (span lifecycle)
//	  → Span (tree of operations)
//	    → Event (timestamped annotations within a span)
//	  → Exporter (pluggable export: memory, OTLP, custom)
//
// The trace model uses LLM-specific SpanKinds (AgentRun, LLMCall, ToolCall, etc.)
// and semantic attributes from OpenTelemetry's gen_ai.* namespace.
package trace

import (
	"time"
)

// TraceID is a globally unique identifier for a trace (16 bytes, hex-encoded).
type TraceID string

// SpanID is a unique identifier for a span within a trace (8 bytes, hex-encoded).
type SpanID string

// SpanKind categorizes the type of operation a span represents.
//
// These kinds are LLM-specific, aligned with OpenTelemetry GenAI semantic conventions
// and OpenClaw's trace hierarchy.
type SpanKind string

const (
	// SpanKindAgentRun is the root span for an entire agent execution.
	// Corresponds to OpenClaw's top-level "agent-run" span.
	SpanKindAgentRun SpanKind = "agent.run"

	// SpanKindAgentTurn is a single conversation turn within an agent run.
	// An agent run may contain multiple turns (e.g., after compaction retry).
	SpanKindAgentTurn SpanKind = "agent.turn"

	// SpanKindLLMCall is a single LLM inference call.
	// Corresponds to OpenTelemetry's gen_ai.client span kind.
	SpanKindLLMCall SpanKind = "llm.call"

	// SpanKindToolCall is a single tool/function invocation.
	// Corresponds to OpenTelemetry's gen_ai.tool span kind.
	SpanKindToolCall SpanKind = "tool.call"

	// SpanKindCompaction is a context compaction operation.
	SpanKindCompaction SpanKind = "agent.compaction"

	// SpanKindSubAgent is a sub-agent spawn and execution.
	SpanKindSubAgent SpanKind = "agent.subagent"

	// SpanKindInternal is a generic internal operation.
	SpanKindInternal SpanKind = "internal"
)

// SpanStatus represents the outcome of a span.
type SpanStatus int

const (
	// SpanStatusUnset indicates the span status has not been explicitly set.
	SpanStatusUnset SpanStatus = iota

	// SpanStatusOK indicates the operation completed successfully.
	SpanStatusOK

	// SpanStatusError indicates the operation failed.
	SpanStatusError
)

// String returns the human-readable name of the span status.
func (s SpanStatus) String() string {
	switch s {
	case SpanStatusOK:
		return "OK"
	case SpanStatusError:
		return "ERROR"
	default:
		return "UNSET"
	}
}

// Span represents a single unit of work within a trace.
//
// Spans form a tree: each span has at most one parent and zero or more children.
// The root span of an agent execution is typically SpanKindAgentRun.
//
// Attribute keys follow OpenTelemetry's gen_ai.* semantic conventions.
type Span struct {
	// TraceID is the trace this span belongs to.
	TraceID TraceID `json:"trace_id"`

	// SpanID is the unique identifier for this span.
	SpanID SpanID `json:"span_id"`

	// ParentSpanID is the parent span's ID (empty for root spans).
	ParentSpanID SpanID `json:"parent_span_id,omitempty"`

	// Name is a human-readable operation name (e.g., "llm.call/openai/gpt-4o").
	Name string `json:"name"`

	// Kind categorizes this span.
	Kind SpanKind `json:"kind"`

	// Status is the outcome of this span.
	Status SpanStatus `json:"status"`

	// StatusMessage provides additional context for error status.
	StatusMessage string `json:"status_message,omitempty"`

	// StartTime is when this span started.
	StartTime time.Time `json:"start_time"`

	// EndTime is when this span ended (zero if still active).
	EndTime time.Time `json:"end_time,omitempty"`

	// Duration is the span's duration (computed from EndTime - StartTime).
	Duration time.Duration `json:"duration,omitempty"`

	// Attributes are key-value pairs describing this span.
	// LLM-specific attributes use the gen_ai.* prefix.
	Attributes map[string]interface{} `json:"attributes,omitempty"`

	// Events are timestamped annotations within this span.
	Events []*SpanEvent `json:"events,omitempty"`
}

// IsRoot returns true if this span has no parent.
func (s *Span) IsRoot() bool {
	return s.ParentSpanID == ""
}

// SetAttribute sets a key-value attribute on the span.
func (s *Span) SetAttribute(key string, value interface{}) {
	if s.Attributes == nil {
		s.Attributes = make(map[string]interface{})
	}
	s.Attributes[key] = value
}

// AddEvent adds a timestamped event to the span.
func (s *Span) AddEvent(name string, attrs map[string]interface{}) {
	s.Events = append(s.Events, &SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attrs,
	})
}

// End marks this span as finished with the given status.
func (s *Span) End(status SpanStatus, statusMessage string) {
	s.EndTime = time.Now()
	s.Duration = s.EndTime.Sub(s.StartTime)
	s.Status = status
	s.StatusMessage = statusMessage
}

// SpanEvent is a timestamped annotation within a span.
//
// Events are used to record notable moments during a span's lifetime,
// such as "first token received" or "tool result returned".
type SpanEvent struct {
	// Name identifies the event (e.g., "gen_ai.content.prompt").
	Name string `json:"name"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Attributes are key-value pairs describing this event.
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// Trace is a complete tree of spans representing a single agent execution.
//
// A trace is identified by its TraceID and contains all spans from the
// root AgentRun down to individual LLM calls and tool invocations.
type Trace struct {
	// TraceID is the unique identifier for this trace.
	TraceID TraceID `json:"trace_id"`

	// RootSpanID is the span ID of the root span.
	RootSpanID SpanID `json:"root_span_id"`

	// Spans contains all spans in this trace, keyed by SpanID.
	Spans map[SpanID]*Span `json:"spans"`

	// StartTime is the earliest span start time.
	StartTime time.Time `json:"start_time"`

	// EndTime is the latest span end time.
	EndTime time.Time `json:"end_time,omitempty"`

	// Resource describes the service producing the trace.
	Resource *Resource `json:"resource,omitempty"`
}

// Resource describes the entity producing telemetry.
// Aligned with OpenTelemetry's Resource concept.
type Resource struct {
	// ServiceName is the logical name of the service (e.g., "echoryn-hivemind").
	ServiceName string `json:"service.name"`

	// ServiceVersion is the service version.
	ServiceVersion string `json:"service.version,omitempty"`

	// Attributes are additional resource attributes.
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}
