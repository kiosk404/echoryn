// Package duckduckgo_websearch implements the "duckduckgo-web-search" built-in plugin.
//
// This plugin provides web search via DuckDuckGo — a privacy-focused search engine
// that requires NO API key or registration. It leverages eino-ext/components/tool/duckduckgo/v2.
//
// This plugin shares the "web-search" slot with gemini-web-search — only one can
// be active at a time, controlled via plugins.slots.web-search in configuration.
//
// Registered capabilities:
//   - Tool: "web_search" — search the web via DuckDuckGo
package duckduckgo_websearch

import (
	"context"
	"fmt"
	"time"

	ddg "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "duckduckgo-web-search"
)

// Config holds the DuckDuckGo web search configuration.
type Config struct {
	// Enabled controls whether the plugin is active.
	Enabled bool `json:"enabled"`

	// MaxResults is the maximum number of search results to return.
	// Default: 5.
	MaxResults int `json:"max_results"`

	// Region constrains search results to a specific region (e.g., "cn-zh", "us-en").
	// Default: "" (no region constraint).
	Region string `json:"region"`

	// TimeoutSeconds is the HTTP request timeout in seconds.
	// Default: 15.
	TimeoutSeconds int `json:"timeout_seconds"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Enabled:        false,
		MaxResults:     5,
		TimeoutSeconds: 15,
	}
}

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "DuckDuckGo Web Search",
		Kind:        "web-search", // Slot: only one web-search plugin can be active.
		Description: "Privacy-focused web search via DuckDuckGo (no API key required)",
	}
}

// duckDuckGoPlugin is the runtime instance.
type duckDuckGoPlugin struct {
	cfg      *Config
	searcher ddg.Search
	handle   plugin.Handle
}

// Factory is the PluginFactory for duckduckgo-web-search.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfgRaw, ok := args["config"]
	if !ok {
		return &duckDuckGoPlugin{cfg: DefaultConfig(), handle: handle}, nil
	}
	cfg, ok := cfgRaw.(*Config)
	if !ok {
		return nil, fmt.Errorf("duckduckgo-web-search: 'config' must be *Config, got %T", cfgRaw)
	}
	return &duckDuckGoPlugin{cfg: cfg, handle: handle}, nil
}

// Name implements plugin.Plugin.
func (p *duckDuckGoPlugin) Name() string {
	return PluginName
}

// Init implements plugin.InitPlugin.
func (p *duckDuckGoPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled {
		logger.Info("[DuckDuckGoSearch] plugin is disabled, skipping tool registration")
		return nil
	}

	api.RegisterTool(plugin.ToolDefinition{
		Name:        "web_search",
		Description: "Search the web for real-time information using DuckDuckGo (privacy-focused, no API key required). Returns search results with titles, snippets and URLs.",
		Parameters: []plugin.ParameterDef{
			{Name: "query", Type: "string", Description: "The search query", Required: true},
		},
		Handler: p.handleWebSearch,
		Category: "web",
	})

	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *duckDuckGoPlugin) Start(_ context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}

	maxResults := p.cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}

	timeout := time.Duration(p.cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	ddgCfg := &ddg.Config{
		MaxResults: maxResults,
		Timeout:    timeout,
	}
	if p.cfg.Region != "" {
		ddgCfg.Region = ddg.Region(p.cfg.Region)
	}

	searcher, err := ddg.NewSearch(context.Background(), ddgCfg)
	if err != nil {
		logger.Warn("[DuckDuckGoSearch] failed to create searcher: %v", err)
		return nil
	}
	p.searcher = searcher

	logger.Info("[DuckDuckGoSearch] started (max_results=%d, region=%q, timeout=%ds)",
		maxResults, p.cfg.Region, p.cfg.TimeoutSeconds)
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *duckDuckGoPlugin) Stop(_ context.Context) error {
	return nil
}

// searchResult is the unified response format (same shape as gemini-web-search).
type searchResult struct {
	Content   string     `json:"content"`
	Citations []citation `json:"citations,omitempty"`
}

type citation struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// handleWebSearch invokes the DuckDuckGo searcher and formats results.
func (p *duckDuckGoPlugin) handleWebSearch(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.searcher == nil {
		return nil, fmt.Errorf("duckduckgo-web-search plugin is not initialized")
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("parameter 'query' is required and must be a non-empty string")
	}

	resp, err := p.searcher.TextSearch(ctx, &ddg.TextSearchRequest{
		Query: query,
	})
	if err != nil {
		return nil, fmt.Errorf("duckduckgo search failed: %w", err)
	}

	// Format as unified search result.
	result := &searchResult{}
	var snippets []string
	for _, r := range resp.Results {
		snippets = append(snippets, fmt.Sprintf("**%s**\n%s", r.Title, r.Summary))
		result.Citations = append(result.Citations, citation{
			URL:   r.URL,
			Title: r.Title,
		})
	}
	result.Content = fmt.Sprintf("Search results for %q:\n\n%s", query, joinSnippets(snippets))

	return result, nil
}

func joinSnippets(snippets []string) string {
	result := ""
	for i, s := range snippets {
		if i > 0 {
			result += "\n\n"
		}
		result += fmt.Sprintf("%d. %s", i+1, s)
	}
	return result
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*duckDuckGoPlugin)(nil)
	_ plugin.InitPlugin      = (*duckDuckGoPlugin)(nil)
	_ plugin.LifecyclePlugin = (*duckDuckGoPlugin)(nil)
)
