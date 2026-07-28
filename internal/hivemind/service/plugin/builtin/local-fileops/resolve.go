package localfileops

import genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"

// PluginName is the unique identifier used by the plugin registry and by
// PluginsOptions.entries["local-fileops"].
const PluginName = "local-fileops"

// ResolveLocalFileOpsConfig extracts local-fileops config from
// PluginsOptions.entries["local-fileops"].config, falling back to
// DefaultConfig() for fields that are missing. Unknown keys are silently
// ignored -- same pattern as ResolveWebFetchConfig.
func ResolveLocalFileOpsConfig(opts *genericoptions.PluginsOptions) *Config {
	cfg := DefaultConfig()
	if opts == nil {
		return cfg
	}
	entry, ok := opts.Entries[PluginName]
	if !ok || entry.Config == nil {
		return cfg
	}

	if v, ok := entry.Config["enabled"].(bool); ok {
		cfg.Enabled = v
	}

	if readRaw, ok := entry.Config["read"].(map[string]interface{}); ok {
		if v, ok := readRaw["enabled"].(bool); ok {
			cfg.Read.Enabled = v
		}
		if rootsRaw, ok := readRaw["allowed_roots"].([]interface{}); ok {
			cfg.Read.AllowedRoots = toStringSlice(rootsRaw)
		}
		if v, ok := readRaw["max_bytes"].(float64); ok && v > 0 {
			cfg.Read.MaxBytes = int64(v)
		}
	}

	if writeRaw, ok := entry.Config["write"].(map[string]interface{}); ok {
		if v, ok := writeRaw["enabled"].(bool); ok {
			cfg.Write.Enabled = v
		}
		if rootsRaw, ok := writeRaw["allowed_roots"].([]interface{}); ok {
			cfg.Write.AllowedRoots = toStringSlice(rootsRaw)
		}
	}

	if raw, ok := entry.Config["deny_exact"].([]interface{}); ok {
		cfg.DenyExact = toStringSlice(raw)
	}
	if raw, ok := entry.Config["deny_prefix"].([]interface{}); ok {
		cfg.DenyPrefix = toStringSlice(raw)
	}

	return cfg
}

func toStringSlice(in []interface{}) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
