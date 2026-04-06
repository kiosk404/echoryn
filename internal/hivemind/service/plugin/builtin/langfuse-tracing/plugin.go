// Package langfusetracing implements the "langfuse-tracing" built-in plugin.
//
// This plugin integrates Langfuse (https://langfuse.com) for LLM application-level
// observability. It leverages eino-ext/callbacks/langfuse to automatically trace
// all Eino graph executions (LLM calls, tool invocations, agent loops, etc.)
// without manual instrumentation.
//
// Architecture:
//
//	Eino Graph Execution
//	  → callbacks.Handler (langfuse)    ← registered via AppendGlobalHandlers
//	    → LangfuseHandler batches events
//	      → Langfuse Server (self-hosted or cloud)
//	        → Dashboard: traces, cost, latency, prompt/completion content
//
// Registered capabilities:
//   - Service: "langfuse-tracer" — manages Langfuse handler lifecycle
//
// Complementary to the existing "diagnostics" plugin:
//   - diagnostics: operational metrics + custom OTEL spans (infrastructure level)
//   - langfuse-tracing: full LLM call chain with prompt/completion content (application level)
package langfusetracing

import (
	"context"
	"time"

	"github.com/cloudwego/eino-ext/callbacks/langfuse"
	"github.com/cloudwego/eino/callbacks"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "langfuse-tracing"
)

// Config holds the Langfuse tracing configuration.
//
// Langfuse is an open-source LLM observability platform that provides:
//   - Full LLM call chain tracing (prompt/completion content, token usage, latency)
//   - Cost tracking and analytics dashboard
//   - Prompt management and evaluation
//
// Integration is via eino-ext/callbacks/langfuse which implements Eino's
// callbacks.Handler interface for automatic instrumentation.
type Config struct {
	// Enabled controls whether Langfuse tracing is active.
	Enabled bool `json:"enabled"`

	// Host is the Langfuse server URL (e.g., "https://cloud.langfuse.com" or self-hosted).
	Host string `json:"host"`

	// PublicKey is the Langfuse project public key (pk-lf-...).
	PublicKey string `json:"public_key"`

	// SecretKey is the Langfuse project secret key (sk-lf-...).
	SecretKey string `json:"secret_key"`

	// SampleRate controls trace sampling (0.0 to 1.0, default 1.0).
	// Set to a lower value in production to reduce overhead.
	SampleRate float64 `json:"sample_rate"`

	// FlushAt is the number of events to buffer before sending a batch.
	// Default: 15.
	FlushAt int `json:"flush_at"`

	// FlushIntervalMs is the flush interval in milliseconds.
	// Default: 500.
	FlushIntervalMs int `json:"flush_interval_ms"`

	// MaskInput controls whether to mask/redact LLM input content.
	// When true, prompt content is not sent to Langfuse (privacy mode).
	MaskInput bool `json:"mask_input"`

	// MaskOutput controls whether to mask/redact LLM output content.
	// When true, completion content is not sent to Langfuse (privacy mode).
	MaskOutput bool `json:"mask_output"`
}

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Langfuse Tracing",
		Kind:        "", // No slot constraint — always active when enabled.
		Description: "LLM application-level observability via Langfuse: full call chain tracing with prompt/completion content, token usage, cost analytics",
	}
}

// langfusePlugin is the runtime instance.
type langfusePlugin struct {
	cfg     *Config
	flusher func() // Langfuse batch flusher, called on shutdown.
}

// Factory is the PluginFactory for langfuse-tracing.
func Factory(args plugin.PluginArgs, _ plugin.Handle) (plugin.Plugin, error) {
	cfgRaw, ok := args["config"]
	if !ok {
		return &langfusePlugin{cfg: &Config{}}, nil
	}
	cfg, ok := cfgRaw.(*Config)
	if !ok {
		return &langfusePlugin{cfg: &Config{}}, nil
	}
	return &langfusePlugin{cfg: cfg}, nil
}

// Name implements plugin.Plugin.
func (p *langfusePlugin) Name() string {
	return PluginName
}

// Init implements plugin.InitPlugin.
func (p *langfusePlugin) Init(api plugin.PluginAPI) error {
	api.RegisterService(plugin.ServiceDefinition{
		Name:  "langfuse-tracer",
		Start: p.startTracer,
		Stop:  p.stopTracer,
	})
	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *langfusePlugin) Start(_ context.Context) error {
	if !p.cfg.Enabled {
		logger.Info("[LangfuseTracing] plugin disabled (langfuse.enabled=false)")
		return nil
	}
	logger.Info("[LangfuseTracing] plugin started (host=%s, sample_rate=%.2f, mask_input=%v, mask_output=%v)",
		p.cfg.Host, p.cfg.SampleRate, p.cfg.MaskInput, p.cfg.MaskOutput)
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *langfusePlugin) Stop(_ context.Context) error {
	if p.flusher != nil {
		p.flusher()
		logger.Info("[LangfuseTracing] flushed remaining traces")
	}
	return nil
}

// startTracer initializes the Langfuse callback handler and registers it globally.
func (p *langfusePlugin) startTracer(_ context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}

	if p.cfg.Host == "" || p.cfg.PublicKey == "" || p.cfg.SecretKey == "" {
		logger.Warn("[LangfuseTracing] disabled: missing host, public_key, or secret_key")
		return nil
	}

	// Build Langfuse config.
	lfConfig := &langfuse.Config{
		Host:      p.cfg.Host,
		PublicKey: p.cfg.PublicKey,
		SecretKey: p.cfg.SecretKey,
	}

	// Apply optional settings.
	if p.cfg.SampleRate > 0 && p.cfg.SampleRate <= 1.0 {
		lfConfig.SampleRate = p.cfg.SampleRate
	}
	if p.cfg.FlushAt > 0 {
		lfConfig.FlushAt = p.cfg.FlushAt
	}
	if p.cfg.FlushIntervalMs > 0 {
		lfConfig.FlushInterval = time.Duration(p.cfg.FlushIntervalMs) * time.Millisecond
	}

	// Privacy: mask input/output content if configured.
	if p.cfg.MaskInput || p.cfg.MaskOutput {
		lfConfig.MaskFunc = func(s string) string {
			return "[REDACTED]"
		}
	}

	// Create handler and register globally.
	// Eino's callback system will automatically invoke this handler for
	// every graph node execution (ChatModel, ToolsNode, Lambda, etc.),
	// creating a full trace hierarchy without manual instrumentation.
	handler, flusher := langfuse.NewLangfuseHandler(lfConfig)
	callbacks.AppendGlobalHandlers(handler)
	p.flusher = flusher

	logger.Info("[LangfuseTracing] registered global Eino callback handler (host=%s, flush_at=%d)",
		p.cfg.Host, lfConfig.FlushAt)
	return nil
}

// stopTracer flushes remaining traces.
func (p *langfusePlugin) stopTracer(_ context.Context) error {
	if p.flusher != nil {
		p.flusher()
	}
	return nil
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*langfusePlugin)(nil)
	_ plugin.InitPlugin      = (*langfusePlugin)(nil)
	_ plugin.LifecyclePlugin = (*langfusePlugin)(nil)
)
