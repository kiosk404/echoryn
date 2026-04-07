package langfusetracing

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// ResolveLangfuseConfig extracts langfuse-tracing config from PluginsOptions.entries["langfuse-tracing"].config,
// falling back to defaults if not specified.
//
// API keys support ${ENV_VAR} expansion via helper.ResolveEnvValue for secure config management.
func ResolveLangfuseConfig(opts *genericoptions.PluginsOptions) *Config {
	cfg := &Config{}
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
	if v, ok := entry.Config["host"]; ok {
		if s, ok := v.(string); ok {
			cfg.Host = s
		}
	}
	if v, ok := entry.Config["public_key"]; ok {
		if s, ok := v.(string); ok {
			cfg.PublicKey = helper.ResolveEnvValue(s)
		}
	}
	if v, ok := entry.Config["secret_key"]; ok {
		if s, ok := v.(string); ok {
			cfg.SecretKey = helper.ResolveEnvValue(s)
		}
	}
	if v, ok := entry.Config["sample_rate"]; ok {
		if f, ok := v.(float64); ok {
			cfg.SampleRate = f
		}
	}
	if v, ok := entry.Config["flush_at"]; ok {
		if f, ok := v.(float64); ok {
			cfg.FlushAt = int(f)
		}
	}
	if v, ok := entry.Config["flush_interval_ms"]; ok {
		if f, ok := v.(float64); ok {
			cfg.FlushIntervalMs = int(f)
		}
	}
	if v, ok := entry.Config["mask_input"]; ok {
		if b, ok := v.(bool); ok {
			cfg.MaskInput = b
		}
	}
	if v, ok := entry.Config["mask_output"]; ok {
		if b, ok := v.(bool); ok {
			cfg.MaskOutput = b
		}
	}

	return cfg
}
