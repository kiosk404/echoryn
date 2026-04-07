// Package langsmithtracing implements the "langsmith-tracing" built-in plugin.
//
// This plugin integrates LangSmith (https://langchain.com/langsmith) for LLM
// application-level observability. It leverages eino-ext/callbacks/langsmith to
// automatically trace all Eino graph executions.
//
// LangSmith is a cloud-only platform (no self-hosting). Use this plugin when
// you prefer the LangChain ecosystem's tracing, evaluation, and dataset features.
//
// This plugin shares the "tracing" slot with langfuse-tracing — only one can
// be active at a time, controlled via plugins.slots.tracing in configuration.
//
// Registered capabilities:
//   - Service: "langsmith-tracer" — manages LangSmith handler lifecycle
package langsmithtracing

import (
	"context"

	"github.com/cloudwego/eino-ext/callbacks/langsmith"
	"github.com/cloudwego/eino/callbacks"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "langsmith-tracing"
)

// Config holds the LangSmith tracing configuration.
type Config struct {
	// Enabled controls whether LangSmith tracing is active.
	Enabled bool `json:"enabled"`

	// APIKey is the LangSmith API key.
	APIKey string `json:"api_key"`

	// APIURL is the LangSmith API endpoint.
	// Default: "https://api.smith.langchain.com" (cloud).
	APIURL string `json:"api_url"`

	// ProjectName is the LangSmith project to report traces to.
	// Default: "default".
	ProjectName string `json:"project_name"`
}

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "LangSmith Tracing",
		Kind:        "tracing", // Slot: only one tracing plugin can be active.
		Description: "LLM application-level observability via LangSmith (cloud): full call chain tracing with prompt/completion content, evaluation, datasets",
	}
}

// langsmithPlugin is the runtime instance.
type langsmithPlugin struct {
	cfg *Config
}

// Factory is the PluginFactory for langsmith-tracing.
func Factory(args plugin.PluginArgs, _ plugin.Handle) (plugin.Plugin, error) {
	cfgRaw, ok := args["config"]
	if !ok {
		return &langsmithPlugin{cfg: &Config{}}, nil
	}
	cfg, ok := cfgRaw.(*Config)
	if !ok {
		return &langsmithPlugin{cfg: &Config{}}, nil
	}
	return &langsmithPlugin{cfg: cfg}, nil
}

// Name implements plugin.Plugin.
func (p *langsmithPlugin) Name() string {
	return PluginName
}

// Init implements plugin.InitPlugin.
func (p *langsmithPlugin) Init(api plugin.PluginAPI) error {
	api.RegisterService(plugin.ServiceDefinition{
		Name:  "langsmith-tracer",
		Start: p.startTracer,
		Stop:  p.stopTracer,
	})
	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *langsmithPlugin) Start(_ context.Context) error {
	if !p.cfg.Enabled {
		logger.Info("[LangSmithTracing] plugin disabled (langsmith.enabled=false)")
		return nil
	}
	logger.Info("[LangSmithTracing] plugin started (api_url=%s, project=%s)",
		p.cfg.APIURL, p.cfg.ProjectName)
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *langsmithPlugin) Stop(_ context.Context) error {
	return nil
}

// startTracer initializes the LangSmith callback handler and registers it globally.
func (p *langsmithPlugin) startTracer(_ context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}

	if p.cfg.APIKey == "" {
		logger.Warn("[LangSmithTracing] disabled: missing api_key")
		return nil
	}

	apiURL := p.cfg.APIURL
	if apiURL == "" {
		apiURL = "https://api.smith.langchain.com"
	}

	handler, err := langsmith.NewLangsmithHandler(&langsmith.Config{
		APIKey: p.cfg.APIKey,
		APIURL: apiURL,
	})
	if err != nil {
		logger.Warn("[LangSmithTracing] failed to create handler: %v", err)
		return nil
	}

	// Register globally — Eino callback system merges with per-call handlers.
	callbacks.AppendGlobalHandlers(handler)

	// Set trace context with project name for session grouping.
	logger.Info("[LangSmithTracing] registered global Eino callback handler (api_url=%s, project=%s)",
		apiURL, p.cfg.ProjectName)
	return nil
}

// stopTracer is a no-op — LangSmith handler has no explicit flush.
func (p *langsmithPlugin) stopTracer(_ context.Context) error {
	return nil
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*langsmithPlugin)(nil)
	_ plugin.InitPlugin      = (*langsmithPlugin)(nil)
	_ plugin.LifecyclePlugin = (*langsmithPlugin)(nil)
)
