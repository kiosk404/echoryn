package feishu

// FeishuConfig holds the configuration for the Feishu channel plugin.
type FeishuConfig struct {
	// Enabled controls whether the Feishu channel is active
	Enabled bool `json:"enabled"`

	// AppID is the Feishu Bot application ID.
	AppID string `json:"app_id"`

	// AppSecret is the Feishu Bot application secret.
	AppSecret string `json:"app_secret"`

	// VerificationToken is used to verify webhook callbacks from Feishu.
	VerificationToken string `json:"verification_token"`

	// EncryptKey is the optional encryption key for Feishu event callbacks.
	EncryptKey string `json:"encrypt_key,omitempty"`

	// AgentID is the agent to route messages to. If empty, uses gateway default.
	AgentID string `json:"agent_id,omitempty"`

	// ListenAddr is the HTTP address for the Feishu webhook server.
	ListenAddr string `json:"listen_addr,omitempty"`

	// WebhookPath is the URL path for the Feishu event callback.
	// Default: "/feishu/event"
	WebhookPath string `json:"webhook_path,omitempty"`
}

// DefaultFeishuConfig returns the default configuration.
func DefaultFeishuConfig() *FeishuConfig {
	return &FeishuConfig{
		Enabled:     false,
		ListenAddr:  ":19801",
		WebhookPath: "/feishu/event",
	}
}
