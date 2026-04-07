package duckduckgo_websearch

import genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"

// ResolveDDGSearchConfig extracts duckduckgo-web-search config from PluginsOptions.entries["duckduckgo-web-search"].config.
func ResolveDDGSearchConfig(opts *genericoptions.PluginsOptions) *Config {
	cfg := DefaultConfig()
	if opts == nil {
		return cfg
	}

	entry, ok := opts.Entries[PluginName]
	if !ok || entry.Config == nil {
		return cfg
	}

	if v, ok := entry.Config["enabled"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Enabled = b
		}
	}
	if v, ok := entry.Config["max_results"]; ok {
		if f, ok := v.(float64); ok {
			cfg.MaxResults = int(f)
		}
	}
	if v, ok := entry.Config["region"]; ok {
		if s, ok := v.(string); ok {
			cfg.Region = s
		}
	}
	if v, ok := entry.Config["timeout_seconds"]; ok {
		if f, ok := v.(float64); ok {
			cfg.TimeoutSeconds = int(f)
		}
	}

	return cfg
}
