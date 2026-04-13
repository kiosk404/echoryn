// Package toolsearch implements the "tool-search" built-in plugin.
//
// This plugin provides tool discovery via keyword search, enabling
// deferred tool loading for large tool sets (MCP servers, cluster tools).
//
// Aligned with Claude Code's ToolSearch mechanism:
//   - Deferred tools only send their name to the LLM (no schema)
//   - The LLM calls tool_search to discover and retrieve full schemas
//   - Reduces initial token consumption from O(N) to O(1) for deferred tools
//
// Query formats:
//   - "select:name1,name2" — exact lookup by tool name
//   - "keyword1 keyword2"  — fuzzy search across names, hints, descriptions
package toolsearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "tool-search"
)

// PluginDefinition returns the static metadata for this plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Tool Search",
		Kind:        "general",
		Description: "Discover available tools by keyword search (enables deferred loading for large tool sets)",
	}
}

// toolSearchPlugin is the runtime instance.
type toolSearchPlugin struct {
	cfg      *Config
	searcher *Searcher
	handle   plugin.Handle
}

// Factory is the PluginFactory for tool-search.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfgRaw, ok := args["config"]
	if !ok {
		return &toolSearchPlugin{cfg: DefaultConfig(), handle: handle}, nil
	}
	cfg, ok := cfgRaw.(*Config)
	if !ok {
		return nil, fmt.Errorf("tool-search: 'config' must be *Config, got %T", cfgRaw)
	}
	return &toolSearchPlugin{cfg: cfg, handle: handle}, nil
}

// Name implements plugin.Plugin.
func (p *toolSearchPlugin) Name() string { return PluginName }

// Init implements plugin.InitPlugin.
func (p *toolSearchPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled {
		logger.Info("[ToolSearch] plugin is disabled, skipping tool registration")
		return nil
	}

	api.RegisterTool(plugin.ToolDefinition{
		Name: "tool_search",
		Description: "Search for available tools by keyword or retrieve specific tools by name. " +
			"Use when you need a tool that is not immediately available in your current tool set. " +
			"Query formats: 'select:name1,name2' (exact lookup), or keywords (fuzzy search).",
		Parameters: []plugin.ParameterDef{
			{Name: "query", Type: "string", Description: "Search query: 'select:tool1,tool2' for exact lookup, or keywords for fuzzy search", Required: true},
		},
		Handler:           p.handleToolSearch,
		AlwaysLoad:        true,
		IsConcurrencySafe: true,
		IsReadOnly:        true,
		Category:          "core",
	})

	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *toolSearchPlugin) Start(_ context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}
	logger.Info("[ToolSearch] started (auto_threshold=%d%%, max_results=%d)",
		p.cfg.AutoThresholdPercent, p.cfg.MaxResults)
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *toolSearchPlugin) Stop(_ context.Context) error { return nil }

// RegistrySetter is probed by server.go to inject the Registry reference
// after framework init (same pattern as GolemDepsSetter).
type RegistrySetter interface {
	SetRegistry(registry *plugin.Registry)
}

// SetRegistry injects the Registry reference after framework init.
func (p *toolSearchPlugin) SetRegistry(registry *plugin.Registry) {
	p.searcher = NewSearcher(registry)
}

// handleToolSearch is the tool handler invoked by agents.
func (p *toolSearchPlugin) handleToolSearch(_ context.Context, params map[string]interface{}) (interface{}, error) {
	if p.searcher == nil {
		return nil, fmt.Errorf("tool-search: searcher not initialized (Registry not injected)")
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("parameter 'query' is required and must be a non-empty string")
	}

	// Mode 1: select:name1,name2 — exact lookup
	if strings.HasPrefix(query, "select:") {
		names := strings.Split(strings.TrimPrefix(query, "select:"), ",")
		tools := p.searcher.SelectByNames(names)
		if len(tools) == 0 {
			return map[string]interface{}{
				"found":   false,
				"message": fmt.Sprintf("No tools found matching names: %s", strings.Join(names, ", ")),
			}, nil
		}
		return p.formatToolSchemas(tools), nil
	}

	// Mode 2: keyword fuzzy search
	results := p.searcher.Search(query, p.cfg.MaxResults)
	if len(results) == 0 {
		return map[string]interface{}{
			"found":   false,
			"message": fmt.Sprintf("No tools found matching query: %q", query),
		}, nil
	}

	return map[string]interface{}{
		"found":   true,
		"count":   len(results),
		"results": results,
	}, nil
}

// formatToolSchemas converts ToolDefinitions to a JSON-friendly schema representation.
func (p *toolSearchPlugin) formatToolSchemas(tools []plugin.ToolDefinition) interface{} {
	type toolSchema struct {
		Name        string                `json:"name"`
		Description string                `json:"description"`
		Parameters  []plugin.ParameterDef `json:"parameters"`
	}

	schemas := make([]toolSchema, 0, len(tools))
	for _, t := range tools {
		schemas = append(schemas, toolSchema{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}

	return map[string]interface{}{
		"found": true,
		"count": len(schemas),
		"tools": schemas,
	}
}

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*toolSearchPlugin)(nil)
	_ plugin.InitPlugin      = (*toolSearchPlugin)(nil)
	_ plugin.LifecyclePlugin = (*toolSearchPlugin)(nil)
)
