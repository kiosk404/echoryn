package builtin

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	memorycore "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core"
	memoryentity "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/entity"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// NewInTreeRegistry creates a new in-tree plugin registry with the default plugins.
// Configuration is sourced from PluginsOptions (plugins.entries.memory-core.config).
// Each plugin receives its config via PluginArgs["config"], resolved from the unified PluginsOptions
// The default plugins are:
// - memory-core: default memory system (SQLite + hybrid search)
func NewInTreeRegistry(opts *genericoptions.PluginsOptions) *plugin.InTreeRegistry {
	registry := plugin.NewInTreeRegistry()

	// --- memory-core: default memory system (SQLite + hybrid search)
	registry.Register(
		memorycore.PluginDefinition(),
		memorycore.Factory,
		plugin.PluginArgs{
			"config": resolveMemoryCoreConfig(opts),
		})

	return registry
}

// resolveMemoryCoreConfig resolves the memory-core plugin config from the given options.
func resolveMemoryCoreConfig(opts *genericoptions.PluginsOptions) *memoryentity.MemoryConfig {
	cfg := memoryentity.DefaultMemoryConfig()
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
				cfg.Embedding.Remote = &memoryentity.RemoteEmbeddingConfig{}
			}
			cfg.Embedding.Remote.APIKey = s
		}
	}
	if v, ok := entry.Config["embedding_base_url"]; ok {
		if s, ok := v.(string); ok {
			if cfg.Embedding.Remote == nil {
				cfg.Embedding.Remote = &memoryentity.RemoteEmbeddingConfig{}
			}
			cfg.Embedding.Remote.BaseURL = s
		}
	}
	return cfg
}
