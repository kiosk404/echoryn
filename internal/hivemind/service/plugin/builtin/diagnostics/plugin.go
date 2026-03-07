// Package diagnostics implements the "diagnostics" built-in plugin.
//
// This plugin provides observability infrastructure for Echoryn's hivemind,
// corresponding to OpenClaw's "diagnostics-otel" plugin.
//
// Registered capabilities:
//   - Hook: HookServerStart / HookServerStop — lifecycle logging
//   - Hook: HookBeforeGenerate / HookAfterGenerate — LLM call tracing (with span creation)
//   - Service: "diagnostics-collector" — background metrics collection
//   - Service: "diagnostics-tracer" — LLM trace engine with pluggable export
//   - Tool: "diagnostics_status" — query system diagnostics (includes trace stats)
//
// The plugin owns:
//   - Collector: in-memory metrics aggregation (counters, histograms)
//   - Tracer: LLM-aware distributed tracing (span lifecycle, batch export)
//
// When OTEL is configured, traces are exported to the OTLP endpoint.
// Otherwise, spans are stored in-memory and accessible via diagnostics_status.
package diagnostics

import (
	"context"
	"fmt"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/diagnostics/collector"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/diagnostics/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/diagnostics/trace"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "diagnostics"
)

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Diagnostics",
		Kind:        "", // No slot constraint — always active.
		Description: "Observability and diagnostics: in-memory metrics, event collection, LLM tracing, and optional OTEL export",
	}
}

// diagnosticsPlugin is the runtime instance.
type diagnosticsPlugin struct {
	cfg            *entity.DiagnosticsConfig
	collector      *collector.Collector
	tracer         *trace.Tracer
	memoryExporter *trace.MemoryExporter
	startTime      time.Time
}

// Factory is the PluginFactory for diagnostics.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfgRaw, ok := args["config"]
	if !ok {
		// Use defaults if no config provided.
		return &diagnosticsPlugin{
			cfg: entity.DefaultDiagnosticsConfig(),
		}, nil
	}
	cfg, ok := cfgRaw.(*entity.DiagnosticsConfig)
	if !ok {
		return nil, fmt.Errorf("diagnostics: 'config' must be *entity.DiagnosticsConfig, got %T", cfgRaw)
	}
	return &diagnosticsPlugin{
		cfg: cfg,
	}, nil
}

// Name implements plugin.Plugin.
func (p *diagnosticsPlugin) Name() string {
	return PluginName
}

// Init implements plugin.InitPlugin.
func (p *diagnosticsPlugin) Init(api plugin.PluginAPI) error {
	// Register diagnostics_status tool (includes trace stats).
	api.RegisterTool(plugin.ToolDefinition{
		Name:        "diagnostics_status",
		Description: "Query system diagnostics: uptime, counters, health metrics, and LLM trace statistics.",
		Parameters:  []plugin.ParameterDef{},
		Handler:     p.handleDiagnosticsStatus,
	})

	// Register lifecycle hooks for tracing.
	api.RegisterHook(plugin.HookServerStart, p.onServerStart)
	api.RegisterHook(plugin.HookServerStop, p.onServerStop)
	api.RegisterHook(plugin.HookBeforeGenerate, p.onBeforeGenerate)
	api.RegisterHook(plugin.HookAfterGenerate, p.onAfterGenerate)

	// Register the collector as a background service.
	api.RegisterService(plugin.ServiceDefinition{
		Name:  "diagnostics-collector",
		Start: p.startCollector,
		Stop:  p.stopCollector,
	})

	// Register the tracer as a background service.
	api.RegisterService(plugin.ServiceDefinition{
		Name:  "diagnostics-tracer",
		Start: p.startTracer,
		Stop:  p.stopTracer,
	})

	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *diagnosticsPlugin) Start(ctx context.Context) error {
	if !p.cfg.Enabled {
		logger.Info("[Diagnostics] diagnostics plugin is disabled")
		return nil
	}

	logger.Info("[Diagnostics] starting diagnostics plugin (otel=%v, traces=%v)",
		p.cfg.OTEL.Enabled, p.cfg.OTEL.Traces)
	p.startTime = time.Now()
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *diagnosticsPlugin) Stop(ctx context.Context) error {
	if p.tracer != nil {
		if err := p.tracer.Shutdown(ctx); err != nil {
			logger.Warn("[Diagnostics] tracer shutdown error: %v", err)
		}
	}
	if p.collector != nil {
		p.collector.Stop()
	}
	if !p.startTime.IsZero() {
		logger.Info("[Diagnostics] diagnostics plugin stopped (uptime=%s)", time.Since(p.startTime))
	}
	return nil
}

// Collector returns the underlying metrics collector (for external integration).
func (p *diagnosticsPlugin) Collector() *collector.Collector {
	return p.collector
}

// Tracer returns the LLM trace engine (for external integration).
// Returns nil if the plugin is not enabled or tracer is not started.
func (p *diagnosticsPlugin) Tracer() *trace.Tracer {
	return p.tracer
}

// MemoryExporter returns the in-memory trace store (for diagnostics_status tool).
// Returns nil if OTLP export is enabled exclusively.
func (p *diagnosticsPlugin) MemoryExporter() *trace.MemoryExporter {
	return p.memoryExporter
}

// --- Service Lifecycle ---

func (p *diagnosticsPlugin) startCollector(ctx context.Context) error {
	p.collector = collector.New()
	p.startTime = time.Now()

	logger.Info("[Diagnostics] collector service started (otel_endpoint=%s, traces=%v, metrics=%v, logs=%v)",
		p.cfg.OTEL.Endpoint, p.cfg.OTEL.Traces, p.cfg.OTEL.Metrics, p.cfg.OTEL.Logs)
	return nil
}

func (p *diagnosticsPlugin) stopCollector(ctx context.Context) error {
	if p.collector != nil {
		p.collector.Stop()
	}
	return nil
}

// startTracer initializes the LLM trace engine with configured exporters.
//
// Exporter resolution:
//   - Always: MemoryExporter (for diagnostics_status tool inspection)
//   - If OTEL traces enabled: add OTLPHTTPExporter
//   - Debug mode: add LogExporter
func (p *diagnosticsPlugin) startTracer(_ context.Context) error {
	// Always create a memory exporter for in-process trace inspection.
	p.memoryExporter = trace.NewMemoryExporter(500)

	var exporters []trace.Exporter
	exporters = append(exporters, p.memoryExporter)

	// Add OTLP exporter if configured.
	if p.cfg.OTEL.Enabled && p.cfg.OTEL.Traces {
		endpoint := p.cfg.OTEL.Endpoint
		if endpoint != "" {
			// Append /v1/traces if not already present.
			otlpExp := trace.NewOTLPHTTPExporter(trace.OTLPHTTPExporterConfig{
				Endpoint: endpoint + "/v1/traces",
				Headers:  p.cfg.OTEL.Headers,
			})
			exporters = append(exporters, otlpExp)
			logger.Info("[Diagnostics] OTLP trace exporter configured: %s", endpoint)
		}
	}

	// Add log exporter for debug visibility.
	exporters = append(exporters, trace.NewLogExporter(false))

	// Build the tracer.
	var exporter trace.Exporter
	if len(exporters) == 1 {
		exporter = exporters[0]
	} else {
		exporter = trace.NewMultiExporter(exporters...)
	}

	sampleRate := p.cfg.OTEL.SampleRate
	if sampleRate <= 0 {
		sampleRate = 1.0
	}

	serviceName := p.cfg.OTEL.ServiceName
	if serviceName == "" {
		serviceName = "echoryn-hivemind"
	}

	p.tracer = trace.NewTracer(trace.TracerConfig{
		ServiceName: serviceName,
		SampleRate:  sampleRate,
		Exporter:    exporter,
	})

	logger.Info("[Diagnostics] tracer service started (sample_rate=%.2f, exporters=%d)", sampleRate, len(exporters))
	return nil
}

func (p *diagnosticsPlugin) stopTracer(ctx context.Context) error {
	if p.tracer != nil {
		return p.tracer.Shutdown(ctx)
	}
	return nil
}

// --- Tool Handlers ---

func (p *diagnosticsPlugin) handleDiagnosticsStatus(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	result := map[string]interface{}{
		"plugin":  PluginName,
		"enabled": p.cfg.Enabled,
		"otel": map[string]interface{}{
			"enabled":  p.cfg.OTEL.Enabled,
			"endpoint": p.cfg.OTEL.Endpoint,
			"traces":   p.cfg.OTEL.Traces,
			"metrics":  p.cfg.OTEL.Metrics,
			"logs":     p.cfg.OTEL.Logs,
		},
	}

	if !p.startTime.IsZero() {
		result["uptime_seconds"] = int64(time.Since(p.startTime).Seconds())
	}

	if p.collector != nil {
		result["counters"] = p.collector.Snapshot()
	}

	// Include trace statistics from the memory exporter.
	if p.memoryExporter != nil {
		traceStats := map[string]interface{}{
			"total_traces": p.memoryExporter.TraceCount(),
			"total_spans":  p.memoryExporter.SpanCount(),
		}

		// Include recent span summaries (last 10).
		recentSpans := p.memoryExporter.RecentSpans(10)
		spanSummaries := make([]map[string]interface{}, 0, len(recentSpans))
		for _, s := range recentSpans {
			summary := map[string]interface{}{
				"trace_id": s.TraceID,
				"span_id":  s.SpanID,
				"name":     s.Name,
				"kind":     s.Kind,
				"status":   s.Status.String(),
				"duration": s.Duration.String(),
				"start":    s.StartTime.Format(time.RFC3339),
			}
			spanSummaries = append(spanSummaries, summary)
		}
		traceStats["recent_spans"] = spanSummaries

		result["traces"] = traceStats
	}

	return result, nil
}

// --- Hook Handlers ---

func (p *diagnosticsPlugin) onServerStart(ctx context.Context, data interface{}) error {
	if p.collector != nil {
		p.collector.Emit(&entity.DiagnosticEvent{
			Type: entity.EventSessionStart,
			Attrs: map[string]interface{}{
				"event": "server_start",
			},
		})
	}
	logger.Info("[Diagnostics] server started event recorded")
	return nil
}

func (p *diagnosticsPlugin) onServerStop(ctx context.Context, data interface{}) error {
	if p.collector != nil {
		p.collector.Emit(&entity.DiagnosticEvent{
			Type: entity.EventSessionEnd,
			Attrs: map[string]interface{}{
				"event":          "server_stop",
				"uptime_seconds": int64(time.Since(p.startTime).Seconds()),
			},
		})
	}
	logger.Info("[Diagnostics] server stop event recorded")
	return nil
}

func (p *diagnosticsPlugin) onBeforeGenerate(ctx context.Context, data interface{}) error {
	if p.collector != nil {
		p.collector.IncrCounter("generate.attempts", 1)
	}

	// Start an LLM call span if a tracer is active in the context.
	// The span will be ended in onAfterGenerate.
	if p.tracer != nil {
		if hookData, ok := data.(map[string]interface{}); ok {
			provider, _ := hookData["provider"].(string)
			model, _ := hookData["model"].(string)
			if provider != "" || model != "" {
				_, span := p.tracer.Start(ctx,
					fmt.Sprintf("llm.call/%s/%s", provider, model),
					trace.SpanKindLLMCall)
				span.SetAttribute(trace.AttrGenAISystem, provider)
				span.SetAttribute(trace.AttrGenAIRequestModel, model)

				// Store span reference in hook data for onAfterGenerate to end it.
				hookData["_trace_span"] = span
			}
		}
	}
	return nil
}

func (p *diagnosticsPlugin) onAfterGenerate(ctx context.Context, data interface{}) error {
	if p.collector != nil {
		p.collector.IncrCounter("generate.completed", 1)
	}

	// End the LLM call span started in onBeforeGenerate.
	if p.tracer != nil {
		if hookData, ok := data.(map[string]interface{}); ok {
			if span, ok := hookData["_trace_span"].(*trace.Span); ok {
				// Record token usage if available.
				if inputTokens, ok := hookData["input_tokens"].(int64); ok {
					span.SetAttribute(trace.AttrGenAIUsageInputTokens, inputTokens)
				}
				if outputTokens, ok := hookData["output_tokens"].(int64); ok {
					span.SetAttribute(trace.AttrGenAIUsageOutputTokens, outputTokens)
				}
				if responseModel, ok := hookData["response_model"].(string); ok {
					span.SetAttribute(trace.AttrGenAIResponseModel, responseModel)
				}

				// Determine status from hook data.
				status := trace.SpanStatusOK
				statusMsg := ""
				if errStr, ok := hookData["error"].(string); ok && errStr != "" {
					status = trace.SpanStatusError
					statusMsg = errStr
				}
				p.tracer.EndSpan(span, status, statusMsg)
			}
		}
	}
	return nil
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*diagnosticsPlugin)(nil)
	_ plugin.InitPlugin      = (*diagnosticsPlugin)(nil)
	_ plugin.LifecyclePlugin = (*diagnosticsPlugin)(nil)
)
