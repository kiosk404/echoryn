package gemini_websearch

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// ResolveWebSearchConfig extracts web-search config from PluginsOptions.entries["web-search"].config,
// falling back to defaults if not specified.
func ResolveWebSearchConfig(opts *genericoptions.PluginsOptions) *Config {
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
	if v, ok := entry.Config["provider"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Provider = s
		}
	}
	if v, ok := entry.Config["timeout_seconds"]; ok {
		if f, ok := v.(float64); ok {
			cfg.TimeoutSeconds = int(f)
		}
	}

	// Resolve nested gemini config.
	if geminiRaw, ok := entry.Config["gemini"]; ok {
		if geminiMap, ok := geminiRaw.(map[string]interface{}); ok {
			if v, ok := geminiMap["api_key"]; ok {
				if s, ok := v.(string); ok {
					cfg.Gemini.APIKey = helper.ResolveEnvValue(s)
				}
			}
			if v, ok := geminiMap["model"]; ok {
				if s, ok := v.(string); ok && s != "" {
					cfg.Gemini.Model = s
				}
			}
		}
	}

	return cfg
}
