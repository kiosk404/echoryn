package entity

import (
	"fmt"
	"time"
)

// ProbeResult represents the result of a model availability probe.
// Modeled after OpenClaw's ProbeResult type.
type ProbeResult struct {
	// OK indicates whether the probe was successful.
	OK bool `json:"ok"`

	// LatencyMs is the round-trip latency in milliseconds, or 0 if the probe failed or was skipped.
	LatencyMs int64 `json:"latency_ms"`

	// Error is the error message if the probe failed.
	Error string `json:"error,omitempty"`

	// Skipped indicates the probe was intentionally skipped.
	Skipped bool `json:"skipped,omitempty"`

	// ProbeType indicates what capability was probed.
	ProbeType ProbeType `json:"probe_type"`

	// Timestamp is when the probe was performed.
	Timestamp time.Time `json:"timestamp"`
}

// ProbeType classifies what aspect of the model was probed.
type ProbeType int32

const (
	// ProbeType_Chat probes basic chat completion capability.
	ProbeType_Chat ProbeType = 0

	// ProbeType_ToolCall probes function/tool calling capability.
	ProbeType_ToolCall ProbeType = 1

	// ProbeType_Vision probes image/vision input capability.
	ProbeType_Vision ProbeType = 2

	// ProbeType_Streaming probes streaming response capability.
	ProbeType_Streaming ProbeType = 3
)

func (p ProbeType) String() string {
	switch p {
	case ProbeType_Chat:
		return "chat"
	case ProbeType_ToolCall:
		return "tool_call"
	case ProbeType_Vision:
		return "vision"
	case ProbeType_Streaming:
		return "streaming"
	default:
		return fmt.Sprintf("ProbeType(%d)", p)
	}
}

// ModelProbeSpec describes the configuration for probing a model.
// This is the input to the probing engine.
type ModelProbeSpec struct {
	// Ref is the model to probe.
	Ref ModelRef `json:"ref"`

	// ProbeTypes specifies which capabilities to probe (empty = probe chat only).
	ProbeTypes []ProbeType `json:"probe_types,omitempty"`

	// TimeoutMs is the per-probe timeout in milliseconds (0 = default 10s).
	TimeoutMs int64 `json:"timeout_ms,omitempty"`
}

// ModelScanResult aggregates all probe results for a single model.
// Modeled after OpenClaw's ModelScanResult.
type ModelScanResult struct {
	// Ref identifies the probed model.
	Ref ModelRef `json:"ref"`

	// Instance is the model instance that was probed.
	Instance *ModelInstance `json:"instance"`

	// Results maps ProbeType to the corresponding result.
	Results map[ProbeType]*ProbeResult `json:"results"`

	// Available is true if at least the basic chat probe succeeded.
	Available bool `json:"available"`

	// ScanTimestamp is when the scan was performed.
	ScanTimestamp time.Time `json:"scan_timestamp"`
}
