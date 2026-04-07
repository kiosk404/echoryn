package golem_cluster

import genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"

// ResolveGolemClusterConfig ...
func ResolveGolemClusterConfig(opts *genericoptions.PluginsOptions) *Config {
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
	return cfg
}
