package llmtask

import (
	llmentity "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/llmtask/entity"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// ResolveLLMTaskConfig extracts llm-task config from PluginsOptions.entries["llm-task"].config,
// falling back to defaults if not specified.
func ResolveLLMTaskConfig(opts *genericoptions.PluginsOptions) *llmentity.LLMTaskConfig {
	cfg := llmentity.DefaultLLMTaskConfig()
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
	if v, ok := entry.Config["default_provider"]; ok {
		if s, ok := v.(string); ok {
			cfg.DefaultProvider = s
		}
	}
	if v, ok := entry.Config["default_model"]; ok {
		if s, ok := v.(string); ok {
			cfg.DefaultModel = s
		}
	}
	if v, ok := entry.Config["max_tokens"]; ok {
		if f, ok := v.(float64); ok {
			cfg.MaxTokens = int(f)
		}
	}
	if v, ok := entry.Config["timeout_ms"]; ok {
		if f, ok := v.(float64); ok {
			cfg.TimeoutMs = int64(f)
		}
	}

	return cfg
}
