package webfetch

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "web-fetch"
)

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Web Fetch",
		Kind:        "general",
		Description: "Fetch and extract readable content from URLs (HTML → markdown/text) with SSRF protection, caching, and invisible-content stripping",
	}
}

// webFetchPlugin is the runtime instance.
type webFetchPlugin struct {
	cfg     *Config
	fetcher *fetcher
	handle  plugin.Handle
}

// Factory is the PluginFactory for web-fetch.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfgRaw, ok := args["config"]
	if !ok {
		return &webFetchPlugin{
			cfg:    DefaultConfig(),
			handle: handle,
		}, nil
	}
	cfg, ok := cfgRaw.(*Config)
	if !ok {
		return nil, fmt.Errorf("web-fetch: 'config' must be *Config, got %T", cfgRaw)
	}
	return &webFetchPlugin{
		cfg:    cfg,
		handle: handle,
	}, nil
}

// Name implements plugin.Plugin.
func (p *webFetchPlugin) Name() string {
	return PluginName
}

// Init implements plugin.InitPlugin.
func (p *webFetchPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled {
		logger.Info("[WebFetch] plugin is disabled, skipping tool registration")
		return nil
	}

	api.RegisterTool(plugin.ToolDefinition{
		Name: "web_fetch",
		Description: "Fetch and extract readable content from a URL (HTML → markdown/text). " +
			"Use for lightweight page access without browser automation. " +
			"Supports HTML, JSON, Markdown, and plain text content types. " +
			"HTML pages are automatically cleaned and converted to readable markdown.",
		Parameters: []plugin.ParameterDef{
			{
				Name:        "url",
				Type:        "string",
				Description: "HTTP or HTTPS URL to fetch",
				Required:    true,
			},
			{
				Name:        "extractMode",
				Type:        "string",
				Description: `Extraction mode: "markdown" (default) or "text". Markdown preserves structure; text strips all formatting.`,
				Required:    false,
			},
			{
				Name:        "maxChars",
				Type:        "number",
				Description: "Maximum characters to return (truncates when exceeded). Minimum: 100.",
				Required:    false,
			},
		},
		Handler: p.handleWebFetch,
		Category: "web",
	})

	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *webFetchPlugin) Start(_ context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}

	p.fetcher = newFetcher(p.cfg)

	logger.Info("[WebFetch] started (maxChars=%d, maxResponseBytes=%d, timeout=%ds, cache=%dm, readability=%v)",
		p.cfg.MaxChars, p.cfg.MaxResponseBytes, p.cfg.TimeoutSeconds,
		p.cfg.CacheTTLMinutes, p.cfg.Readability)
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *webFetchPlugin) Stop(_ context.Context) error {
	logger.Info("[WebFetch] stopped")
	return nil
}

// handleWebFetch is the tool handler invoked by agents.
func (p *webFetchPlugin) handleWebFetch(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if p.fetcher == nil {
		return nil, fmt.Errorf("web-fetch plugin is not initialized (check configuration)")
	}

	// Parse URL parameter (required).
	rawURL, ok := params["url"].(string)
	if !ok || rawURL == "" {
		return nil, fmt.Errorf("parameter 'url' is required and must be a non-empty string")
	}

	// Parse extractMode parameter (optional).
	mode := extractMarkdown
	if modeStr, ok := params["extractMode"].(string); ok {
		switch modeStr {
		case "text":
			mode = extractText
		case "markdown", "":
			mode = extractMarkdown
		default:
			return nil, fmt.Errorf("parameter 'extractMode' must be \"markdown\" or \"text\", got %q", modeStr)
		}
	}

	// Parse maxChars parameter (optional).
	maxChars := 0
	if v, ok := params["maxChars"]; ok {
		switch n := v.(type) {
		case float64:
			maxChars = int(n)
		case int:
			maxChars = n
		}
	}

	result, err := p.fetcher.fetch(ctx, rawURL, mode, maxChars)
	if err != nil {
		return nil, fmt.Errorf("web fetch failed: %w", err)
	}

	return result, nil
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*webFetchPlugin)(nil)
	_ plugin.InitPlugin      = (*webFetchPlugin)(nil)
	_ plugin.LifecyclePlugin = (*webFetchPlugin)(nil)
)
