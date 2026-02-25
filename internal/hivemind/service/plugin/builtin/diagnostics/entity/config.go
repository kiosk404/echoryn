package entity

import (
	"time"
)

// DiagnosticsConfig is the resolved configuration for the diagnostics plugin.
// This corresponds to OpenClaw's diagnostics-otel plugin configuration.
type DiagnosticsConfig struct {
	// Enabled controls whether the diagnostics system is active.
	Enabled bool `json:"enabled"`

	// OTEL holds the OpenTelemetry exporter configuration.
	OTEL OTELConfig `json:"otel"`
}

// OTELConfig holds the OpenTelemetry exporter settings.
type OTELConfig struct {
	// Enabled controls whether OTEL export is active.
	Enabled bool `json:"enabled"`

	// Endpoint is the OTLP HTTP endpoint (e.g., "http://localhost:4318").
	Endpoint string `json:"endpoint"`

	// Protocol is the OTLP transport protocol: "http/protobuf" or "grpc".
	Protocol string `json:"protocol"`

	// Headers are custom HTTP headers sent with OTLP requests.
	Headers map[string]string `json:"headers,omitempty"`

	// ServiceName is the OTEL service.name resource attribute.
	ServiceName string `json:"service_name"`

	// SampleRate controls trace sampling (0.0 to 1.0, default 1.0).
	SampleRate float64 `json:"sample_rate"`

	// Traces enables trace export.
	Traces bool `json:"traces"`

	// Metrics enables metric export.
	Metrics bool `json:"metrics"`

	// Logs enables log export.
	Logs bool `json:"logs"`

	// MetricInterval is the metric export interval.
	MetricInterval time.Duration `json:"metric_interval"`
}

// DefaultDiagnosticsConfig returns a sensible default diagnostics configuration.
func DefaultDiagnosticsConfig() *DiagnosticsConfig {
	return &DiagnosticsConfig{
		Enabled: false,
		OTEL: OTELConfig{
			Enabled:        false,
			Endpoint:       "http://localhost:4318",
			Protocol:       "http/protobuf",
			ServiceName:    "echoryn-hivemind",
			SampleRate:     1.0,
			Traces:         true,
			Metrics:        true,
			Logs:           true,
			MetricInterval: 30 * time.Second,
		},
	}
}
