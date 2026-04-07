package langsmithtracing

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// ResolveLangSmithConfig extracts langsmith-tracing config from PluginsOptions.entries["langsmith-tracing"].config.
func ResolveLangSmithConfig(opts *genericoptions.PluginsOptions) *Config {
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
	if v, ok := entry.Config["api_key"]; ok {
		if s, ok := v.(string); ok {
			cfg.APIKey = helper.ResolveEnvValue(s)
		}
	}
	if v, ok := entry.Config["api_url"]; ok {
		if s, ok := v.(string); ok {
			cfg.APIURL = s
		}
	}
	if v, ok := entry.Config["project_name"]; ok {
		if s, ok := v.(string); ok {
			cfg.ProjectName = s
		}
	}

	return cfg
}
