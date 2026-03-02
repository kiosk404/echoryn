package telegram

// TelegramConfig holds the configuration for the Telegram channel plugin.
type TelegramConfig struct {
	// Enabled controls whether the Telegram channel is active.
	Enabled bool `json:"enabled"`

	// BotToken is the Telegram Bot API token (from @BotFather)
	BotToken string `json:"bot_token"`

	// AgentID is the agent to route messages to, If empty, uses gateway default.
	AgentID string `json:"agent_id,omitempty"`

	// PollingTimeout is the long polling timeout in seconds.
	PollingTimeout int `json:"polling_timeout,omitempty"`

	// AllowedChatIDs restricts the bot to specific chat IDs.
	AllowedChatIDs []int64 `json:"allowed_chat_ids,omitempty"`
}

// DefaultTelegramConfig returns the default configuration
func DefaultTelegramConfig() *TelegramConfig {
	return &TelegramConfig{
		Enabled:        false,
		PollingTimeout: 30,
	}
}
