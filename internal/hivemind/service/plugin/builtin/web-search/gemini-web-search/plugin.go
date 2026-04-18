package gemini_websearch

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "gemini-web-search"
)

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Gemini Web Search",
		Kind:        "web-search",
		Description: "Web search tool using Gemini with Google Search grounding for real-time information retrieval",
	}
}

// webSearchPlugin is the runtime instance.
type webSearchPlugin struct {
	cfg      *Config
	searcher *geminiSearcher
	handle   plugin.Handle
}

// Factory is the PluginFactory for web-search.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfgRaw, ok := args["config"]
	if !ok {
		return &webSearchPlugin{
			cfg:    DefaultConfig(),
			handle: handle,
		}, nil
	}
	cfg, ok := cfgRaw.(*Config)
	if !ok {
		return nil, fmt.Errorf("web-search: 'config' must be *Config, got %T", cfgRaw)
	}
	return &webSearchPlugin{
		cfg:    cfg,
		handle: handle,
	}, nil
}

// Name implements plugin.Plugin.
func (p *webSearchPlugin) Name() string {
	return PluginName
}

// Init implements plugin.InitPlugin.
func (p *webSearchPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled {
		logger.Info("[WebSearch] plugin is disabled, skipping tool registration")
		return nil
	}

	api.RegisterTool(plugin.ToolDefinition{
		Name:        "web_search",
		Description: "Search the web for real-time information using Google Search grounding. Returns AI-synthesized answers with citations from current web sources. Use this when you need up-to-date information, news, or facts that may not be in your training data.",
		Parameters: []plugin.ParameterDef{
			{Name: "query", Type: "string", Description: "The search query to look up on the web", Required: true},
		},
		Handler: p.handleWebSearch,
		Category: "web",
	})

	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *webSearchPlugin) Start(ctx context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}

	searcher, err := newGeminiSearcher(p.cfg)
	if err != nil {
		logger.Warn("[GeminiWebSearch] failed to initialize searcher: %v", err)
		return nil
	}
	p.searcher = searcher

	logger.Info("[GeminiWebSearch] started (provider=%s, model=%s, timeout=%ds)",
		p.cfg.Provider, p.cfg.Gemini.Model, p.cfg.TimeoutSeconds)
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *webSearchPlugin) Stop(ctx context.Context) error {
	logger.Info("[WebSearch] stopped")
	return nil
}

// handleWebSearch is the tool handler invoked by agents.
func (p *webSearchPlugin) handleWebSearch(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.searcher == nil {
		return nil, fmt.Errorf("web-search plugin is not initialized (check API key configuration)")
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("parameter 'query' is required and must be a non-empty string")
	}

	result, err := p.searcher.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("web search failed: %w", err)
	}

	return result, nil
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*webSearchPlugin)(nil)
	_ plugin.InitPlugin      = (*webSearchPlugin)(nil)
	_ plugin.LifecyclePlugin = (*webSearchPlugin)(nil)
)
