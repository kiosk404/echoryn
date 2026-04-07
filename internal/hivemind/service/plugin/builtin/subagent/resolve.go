package subagent

import (
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// ResolveSubAgentConfig extracts subagent config from PluginsOptions.entries["subagent"].config,
// falling back to defaults if not specified.
func ResolveSubAgentConfig(opts *genericoptions.PluginsOptions) *Config {
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
	if v, ok := entry.Config["max_concurrent"]; ok {
		if f, ok := v.(float64); ok {
			cfg.MaxConcurrent = int(f)
		}
	}
	if v, ok := entry.Config["archive_after_minutes"]; ok {
		if f, ok := v.(float64); ok {
			cfg.ArchiveAfterMinutes = int(f)
		}
	}

	return cfg
}
