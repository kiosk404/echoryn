package memory_core

import (
	mementity "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory/memory-core/entity"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
	"github.com/kiosk404/echoryn/pkg/paths"
)

// ResolveMemoryConfig extracts memory-core config from PluginsOptions.entries["memory-core"].config,
// falling back to DefaultMemoryConfig if not specified.
//
// Path fields (workspace_dir, db_path) default to ~/.echoryn paths via DefaultMemoryConfig().
func ResolveMemoryConfig(opts *genericoptions.PluginsOptions) *mementity.MemoryConfig {
	cfg := mementity.DefaultMemoryConfig()
	if opts == nil {
		return cfg
	}

	entry, ok := opts.Entries[PluginName]
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
		if s, ok := v.(string); ok && s != "" && s != "." {
			cfg.WorkspaceDir = paths.ResolveWorkspaceDir("", s)
		}
	}
	if v, ok := entry.Config["db_path"]; ok {
		if s, ok := v.(string); ok && s != "" && s != "." {
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
