package hivemind

import (
	"github.com/kiosk404/echoryn/internal/hivemind/handler/middleware"
)

// GatewayConfig holds the gateway-level configuration for HTTP API endpoints.
type GatewayConfig struct {
	// Auth holds the authentication configuration for the gateway.
	Auth middleware.AuthConfig `json:"auth"`
	// Store holds the store configuration for the gateway.
	Store StoreConfig `json:"store"`
	// Defaults holds the default values for the gateway.
	Defaults GatewayDefaults `json:"defaults"`
}

// StoreConfig configures the persistence backend
type StoreConfig struct {
	// Type holds the type of store to use.
	Type string `json:"type"`
	// BoltDBPath holds the path to the BoltDB file.
	BoltDBPath string `json:"bolt_db_path"`
}

// GatewayDefaults holds the default values for the gateway.
type GatewayDefaults struct {
	// AgentID holds the default agent ID to use.
	AgentID string `json:"agent_id"`
	// Model holds the default model to use.
	Model string `json:"model"`
}

func DefaultGatewayConfig() *GatewayConfig {
	return &GatewayConfig{
		Auth: middleware.AuthConfig{
			Enabled: false,
		},
		Store: StoreConfig{
			Type:       "boltdb",
			BoltDBPath: "data/hivemind.db",
		},
		Defaults: GatewayDefaults{
			AgentID: "main",
			Model:   "Echoryn",
		},
	}
}
