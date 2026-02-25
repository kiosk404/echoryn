// Package diagnostics implements the "diagnostics" built-in plugin.
//
// This plugin provides observability infrastructure for Echoryn's hivemind,
// corresponding to OpenClaw's "diagnostics-otel" plugin.
//
// Registered capabilities:
//   - Hook: HookServerStart / HookServerStop — lifecycle logging
//   - Hook: HookBeforeGenerate / HookAfterGenerate — LLM call tracing
//   - Service: "diagnostics-collector" — background metrics collection
//   - Tool: "diagnostics_status" — query system diagnostics
//
// The plugin owns the Collector (in-memory metrics aggregation) and
// can optionally export to OpenTelemetry when configured.
package diagnostics

import (
	"context"
	"fmt"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/diagnostics/collector"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/diagnostics/entity"
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
		Description: "Observability and diagnostics: in-memory metrics, event collection, and optional OTEL export",
	}
}

// diagnosticsPlugin is the runtime instance.
type diagnosticsPlugin struct {
	cfg       *entity.DiagnosticsConfig
	collector *collector.Collector
	startTime time.Time
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
	// Register diagnostics_status tool.
	api.RegisterTool(plugin.ToolDefinition{
		Name:        "diagnostics_status",
		Description: "Query system diagnostics: uptime, counters, and health metrics.",
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

	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *diagnosticsPlugin) Start(ctx context.Context) error {
	if !p.cfg.Enabled {
		logger.Info("[Diagnostics] diagnostics plugin is disabled")
		return nil
	}

	logger.Info("[Diagnostics] starting diagnostics plugin (otel=%v)", p.cfg.OTEL.Enabled)
	p.startTime = time.Now()
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *diagnosticsPlugin) Stop(ctx context.Context) error {
	if p.collector != nil {
		p.collector.Stop()
		logger.Info("[Diagnostics] diagnostics plugin stopped (uptime=%s)", time.Since(p.startTime))
	}
	return nil
}

// Collector returns the underlying metrics collector (for external integration).
func (p *diagnosticsPlugin) Collector() *collector.Collector {
	return p.collector
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
	return nil
}

func (p *diagnosticsPlugin) onAfterGenerate(ctx context.Context, data interface{}) error {
	if p.collector != nil {
		p.collector.IncrCounter("generate.completed", 1)
	}
	return nil
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*diagnosticsPlugin)(nil)
	_ plugin.InitPlugin      = (*diagnosticsPlugin)(nil)
	_ plugin.LifecyclePlugin = (*diagnosticsPlugin)(nil)
)
