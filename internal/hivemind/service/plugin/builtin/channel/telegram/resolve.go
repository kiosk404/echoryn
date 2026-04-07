package telegram

import genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"

// ResolveTelegramConfig extracts channel-telegram config from PluginsOptions.entries["channel-telegram"].config,
// falling back to defaults if not specified.
func ResolveTelegramConfig(opts *genericoptions.PluginsOptions) *TelegramConfig {
	cfg := DefaultTelegramConfig()
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
	if v, ok := entry.Config["bot_token"]; ok {
		if s, ok := v.(string); ok {
			cfg.BotToken = s
		}
	}
	if v, ok := entry.Config["agent_id"]; ok {
		if s, ok := v.(string); ok {
			cfg.AgentID = s
		}
	}
	if v, ok := entry.Config["polling_timeout"]; ok {
		if f, ok := v.(float64); ok {
			cfg.PollingTimeout = int(f)
		}
	}

	return cfg
}
