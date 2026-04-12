package webfetch

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// ResolveWebFetchConfig extracts web-fetch config from PluginsOptions.entries["web-fetch"].config,
// falling back to defaults if not specified.
func ResolveWebFetchConfig(opts *genericoptions.PluginsOptions) *Config {
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
	if v, ok := entry.Config["max_chars"]; ok {
		if f, ok := v.(float64); ok && f >= 100 {
			cfg.MaxChars = int(f)
		}
	}
	if v, ok := entry.Config["max_chars_cap"]; ok {
		if f, ok := v.(float64); ok && f >= 100 {
			cfg.MaxCharsCap = int(f)
		}
	}
	if v, ok := entry.Config["max_response_bytes"]; ok {
		if f, ok := v.(float64); ok {
			val := int(f)
			if val < minMaxResponseBytes {
				val = minMaxResponseBytes
			}
			if val > maxMaxResponseBytes {
				val = maxMaxResponseBytes
			}
			cfg.MaxResponseBytes = val
		}
	}
	if v, ok := entry.Config["timeout_seconds"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			cfg.TimeoutSeconds = int(f)
		}
	}
	if v, ok := entry.Config["cache_ttl_minutes"]; ok {
		if f, ok := v.(float64); ok && f >= 0 {
			cfg.CacheTTLMinutes = int(f)
		}
	}
	if v, ok := entry.Config["max_redirects"]; ok {
		if f, ok := v.(float64); ok && f >= 0 {
			cfg.MaxRedirects = int(f)
		}
	}
	if v, ok := entry.Config["user_agent"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.UserAgent = s
		}
	}
	if v, ok := entry.Config["readability"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Readability = b
		}
	}

	// Resolve nested firecrawl config.
	if fcRaw, ok := entry.Config["firecrawl"]; ok {
		if fcMap, ok := fcRaw.(map[string]interface{}); ok {
			fc := &FirecrawlConfig{
				OnlyMainContent: true, // default
				MaxAgeMs:        defaultFirecrawlMaxAgeMs,
				TimeoutSeconds:  defaultFirecrawlTimeoutSeconds,
			}
			if v, ok := fcMap["enabled"]; ok {
				if b, ok := v.(bool); ok {
					fc.Enabled = b
				}
			}
			if v, ok := fcMap["api_key"]; ok {
				if s, ok := v.(string); ok {
					fc.APIKey = helper.ResolveEnvValue(s)
				}
			}
			if v, ok := fcMap["base_url"]; ok {
				if s, ok := v.(string); ok && s != "" {
					fc.BaseURL = s
				}
			}
			if v, ok := fcMap["only_main_content"]; ok {
				if b, ok := v.(bool); ok {
					fc.OnlyMainContent = b
				}
			}
			if v, ok := fcMap["max_age_ms"]; ok {
				if f, ok := v.(float64); ok && f >= 0 {
					fc.MaxAgeMs = int64(f)
				}
			}
			if v, ok := fcMap["timeout_seconds"]; ok {
				if f, ok := v.(float64); ok && f > 0 {
					fc.TimeoutSeconds = int(f)
				}
			}
			// Auto-enable if API key is provided.
			if fc.APIKey != "" && !fc.Enabled {
				fc.Enabled = true
			}
			cfg.Firecrawl = fc
		}
	}

	return cfg
}
