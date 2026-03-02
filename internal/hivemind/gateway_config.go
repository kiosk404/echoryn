package hivemind

import (
	"github.com/kiosk404/echoryn/internal/hivemind/handler/middleware"
	"github.com/kiosk404/echoryn/pkg/paths"
)

// GatewayConfig holds the gateway-level configuration for HTTP API endpoints.
type GatewayConfig struct {
	// Auth holds the authentication configuration for the gateway.
	Auth middleware.AuthConfig `json:"auth"`
	// Store holds the store configuration for the gateway.
	Store StoreConfig `json:"store"`
	// Defaults holds the default values for the gateway.
	Defaults GatewayDefaults `json:"defaults"`
	// Channels holds the IM channel gateway configuration.
	Channels ChannelsConfig `json:"channels"`
}

// ChannelsConfig holds the configuration for IM channel integrations.
type ChannelsConfig struct {
	// Enabled controls whether the IM channel gateway is active.
	Enabled bool `json:"enabled"`
	// DefaultAgentID is the fallback agent for channels that don't specify one.
	DefaultAgentID string `json:"default_agent_id,omitempty"`
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
			BoltDBPath: paths.ResolveSessionStorePath(paths.DefaultAgentID()),
		},
		Defaults: GatewayDefaults{
			AgentID: paths.DefaultAgentID(),
			Model:   "Echoryn",
		},
	}
}
