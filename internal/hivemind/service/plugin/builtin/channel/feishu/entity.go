package feishu

// ConnectionMode defines how to receive events from Feishu.
type ConnectionMode string

const (
	// ConnectionModeWebsocket use Websocket long connection (default, no Public IP required)
	ConnectionModeWebsocket ConnectionMode = "websocket"
	// ConnectionModeWebhook uses HTTP webhook (requires public IP or tunnel ).
	ConnectionModeWebhook ConnectionMode = "webhook"
)

// DomainType defines the Feishu/Lark domain
type DomainType string

const (
	// DomainFeishu is the default Feishu domain (China).
	DomainFeishu DomainType = "feishu"
	// DomainLark is the international Lark domain ()
	DomainLark DomainType = "lark"
)

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

	// ConnectionMode determines how to receive events from Feishu
	// Options: "websocket" (default, no public IP required) or "webhook" (requires public IP).
	// Default: "websocket"
	ConnectionMode ConnectionMode `json:"connection_mode,omitempty"`

	// Domain specifies the Feishu/Lark domain.
	// Options "feishu" (default, China) or "Lark" (international)
	// Default "feishu"
	Domain DomainType `json:"domain,omitempty"`

	// ListenAddr is the HTTP address for the Feishu webhook server.
	ListenAddr string `json:"listen_addr,omitempty"`

	// WebhookPath is the URL path for the Feishu event callback.
	// Default: "/feishu/event"
	WebhookPath string `json:"webhook_path,omitempty"`
}

// DefaultFeishuConfig returns the default configuration.
func DefaultFeishuConfig() *FeishuConfig {
	return &FeishuConfig{
		Enabled:        false,
		ConnectionMode: ConnectionModeWebsocket,
		Domain:         DomainFeishu,
		ListenAddr:     ":19801",
		WebhookPath:    "/feishu/event",
	}
}
