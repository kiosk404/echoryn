// Package builtin registers all in-tree (built-in) plugins.
// This is analogous to K8s scheduler's algorithmprovider package,
// which registers all default scheduling plugins.
//
// New built-in plugins should be added to NewInTreeRegistry().
package builtin

import (
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/diagnostics"
	diagentity "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/diagnostics/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/llmtask"
	llmentity "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/llmtask/entity"
	memorycore "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core"
	mementity "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/subagent"
)

// NewInTreeRegistry creates the in-tree plugin registry with all
// built-in plugins pre-registered.
//
// Configuration is sourced from PluginsOptions (aligned with OpenClaw's
// plugins.entries[pluginID].config pattern). Each plugin receives its
// config via PluginArgs["config"], resolved from the unified PluginsOptions.
//
// This is the single source of truth for what plugins ship with Echoryn.
// To add a new built-in plugin:
// 1. Create the plugin package under builtin/
// 2. Add a Register call here
func NewInTreeRegistry(opts *genericoptions.PluginsOptions) *plugin.InTreeRegistry {
	registry := plugin.NewInTreeRegistry()

	// --- memory-core: default memory system (SQLite + hybrid search) ---
	registry.Register(
		memorycore.PluginDefinition(),
		memorycore.Factory,
		plugin.PluginArgs{
			"config": resolveMemoryConfig(opts),
		},
	)

	// --- diagnostics: observability and metrics collection ---
	registry.Register(
		diagnostics.PluginDefinition(),
		diagnostics.Factory,
		plugin.PluginArgs{
			"config": resolveDiagnosticsConfig(opts),
		},
	)

	// --- llm-task: generic JSON-only LLM tool ---
	registry.Register(
		llmtask.PluginDefinition(),
		llmtask.Factory,
		plugin.PluginArgs{
			"config": resolveLLMTaskConfig(opts),
		},
	)

	// --- subagent: sub-agent orchestration (sessions_spawn/sessions_status) ---
	registry.Register(
		subagent.PluginDefinition(),
		subagent.Factory,
		plugin.PluginArgs{
			"config": resolveSubAgentConfig(opts),
		},
	)

	return registry
}

// resolveMemoryConfig extracts memory-core config from PluginsOptions.entries["memory-core"].config,
// falling back to DefaultMemoryConfig if not specified.
func resolveMemoryConfig(opts *genericoptions.PluginsOptions) *mementity.MemoryConfig {
	cfg := mementity.DefaultMemoryConfig()
	if opts == nil {
		return cfg
	}

	entry, ok := opts.Entries[memorycore.PluginName]
	if !ok || entry.Config == nil {
		return cfg
	}

	// Apply user overrides from plugins.entries.memory-core.config.
	if v, ok := entry.Config["enabled"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Enabled = b
		}
	}
	if v, ok := entry.Config["workspace_dir"]; ok {
		if s, ok := v.(string); ok {
			cfg.WorkspaceDir = s
		}
	}
	if v, ok := entry.Config["db_path"]; ok {
		if s, ok := v.(string); ok {
			cfg.Store.Path = s
		}
	}
	if v, ok := entry.Config["embedding_provider"]; ok {
		if s, ok := v.(string); ok {
			cfg.Embedding.Provider = s
		}
	}
	if v, ok := entry.Config["embedding_model"]; ok {
		if s, ok := v.(string); ok {
			cfg.Embedding.Model = s
		}
	}
	if v, ok := entry.Config["embedding_api_key"]; ok {
		if s, ok := v.(string); ok {
			if cfg.Embedding.Remote == nil {
				cfg.Embedding.Remote = &mementity.RemoteEmbeddingConfig{}
			}
			cfg.Embedding.Remote.APIKey = s
		}
	}
	if v, ok := entry.Config["embedding_base_url"]; ok {
		if s, ok := v.(string); ok {
			if cfg.Embedding.Remote == nil {
				cfg.Embedding.Remote = &mementity.RemoteEmbeddingConfig{}
			}
			cfg.Embedding.Remote.BaseURL = s
		}
	}

	return cfg
}

// resolveDiagnosticsConfig extracts diagnostics config from PluginsOptions.entries["diagnostics"].config,
// falling back to defaults if not specified.
func resolveDiagnosticsConfig(opts *genericoptions.PluginsOptions) *diagentity.DiagnosticsConfig {
	cfg := diagentity.DefaultDiagnosticsConfig()
	if opts == nil {
		return cfg
	}

	entry, ok := opts.Entries[diagnostics.PluginName]
	if !ok || entry.Config == nil {
		return cfg
	}

	if v, ok := entry.Config["enabled"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Enabled = b
		}
	}
	if v, ok := entry.Config["otel_endpoint"]; ok {
		if s, ok := v.(string); ok {
			cfg.OTEL.Endpoint = s
		}
	}
	if v, ok := entry.Config["otel_enabled"]; ok {
		if b, ok := v.(bool); ok {
			cfg.OTEL.Enabled = b
		}
	}
	if v, ok := entry.Config["enable_traces"]; ok {
		if b, ok := v.(bool); ok {
			cfg.OTEL.Traces = b
		}
	}
	if v, ok := entry.Config["enable_metrics"]; ok {
		if b, ok := v.(bool); ok {
			cfg.OTEL.Metrics = b
		}
	}
	if v, ok := entry.Config["enable_logs"]; ok {
		if b, ok := v.(bool); ok {
			cfg.OTEL.Logs = b
		}
	}
	if v, ok := entry.Config["service_name"]; ok {
		if s, ok := v.(string); ok {
			cfg.OTEL.ServiceName = s
		}
	}

	return cfg
}

// resolveLLMTaskConfig extracts llm-task config from PluginsOptions.entries["llm-task"].config,
// falling back to defaults if not specified.
func resolveLLMTaskConfig(opts *genericoptions.PluginsOptions) *llmentity.LLMTaskConfig {
	cfg := llmentity.DefaultLLMTaskConfig()
	if opts == nil {
		return cfg
	}

	entry, ok := opts.Entries[llmtask.PluginName]
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

// resolveSubAgentConfig extracts subagent config from PluginsOptions.entries["subagent"].config,
// falling back to defaults if not specified.
func resolveSubAgentConfig(opts *genericoptions.PluginsOptions) *subagent.Config {
	cfg := subagent.DefaultConfig()
	if opts == nil {
		return cfg
	}

	entry, ok := opts.Entries[subagent.PluginName]
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
