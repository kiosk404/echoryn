package toolsearch

import (
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// Config holds the tool-search plugin configuration.
type Config struct {
	// Enabled controls whether the plugin is active. Default: true.
	Enabled bool `json:"enabled"`

	// AutoThresholdPercent is the context window percentage threshold
	// at which ToolSearch automatically enables deferral for all tools.
	// Default: 10 (when tools consume >10% of context window).
	AutoThresholdPercent int `json:"auto_threshold_percent"`

	// MaxResults is the maximum number of search results to return.
	// Default: 10.
	MaxResults int `json:"max_results"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Enabled:              true,
		AutoThresholdPercent: 10,
		MaxResults:           10,
	}
}

// ResolveToolSearchConfig resolves the tool-search config from PluginsOptions.
func ResolveToolSearchConfig(opts *genericoptions.PluginsOptions) *Config {
	cfg := DefaultConfig()
	if opts == nil {
		return cfg
	}
	entry, ok := opts.Entries["tool-search"]
	if !ok {
		return cfg
	}
	if entry.Enabled != nil {
		cfg.Enabled = *entry.Enabled
	}
	if v, ok := entry.Config["auto_threshold_percent"].(float64); ok {
		cfg.AutoThresholdPercent = int(v)
	}
	if v, ok := entry.Config["max_results"].(float64); ok {
		cfg.MaxResults = int(v)
	}
	return cfg
}
