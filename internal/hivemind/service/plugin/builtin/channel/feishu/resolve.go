package feishu

import (
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// ResolveFeishuConfig extracts channel-feishu config from PluginsOptions.entries["channel-feishu"].config,
// falling back to defaults if not specified.
func ResolveFeishuConfig(opts *genericoptions.PluginsOptions) *FeishuConfig {
	cfg := DefaultFeishuConfig()
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
	if v, ok := entry.Config["app_id"]; ok {
		if b, ok := v.(string); ok {
			cfg.AppID = helper.ResolveEnvValue(b)
		}
	}
	if v, ok := entry.Config["app_secret"]; ok {
		if s, ok := v.(string); ok {
			cfg.AppSecret = helper.ResolveEnvValue(s)
		}
	}
	if v, ok := entry.Config["verification_token"]; ok {
		if s, ok := v.(string); ok {
			cfg.VerificationToken = s
		}
	}
	if v, ok := entry.Config["encrypt_key"]; ok {
		if s, ok := v.(string); ok {
			cfg.EncryptKey = s
		}
	}
	if v, ok := entry.Config["agent_id"]; ok {
		if s, ok := v.(string); ok {
			cfg.AgentID = s
		}
	}
	if v, ok := entry.Config["listen_addr"]; ok {
		if s, ok := v.(string); ok {
			cfg.ListenAddr = s
		}
	}
	if v, ok := entry.Config["webhook_path"]; ok {
		if s, ok := v.(string); ok {
			cfg.WebhookPath = s
		}
	}

	return cfg
}
