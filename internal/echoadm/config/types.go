package config

// EchorynConfig is the top-level configuration stored in ~/.echoryn/config.json.
// Hivemind role: ~/.echoryn/hivemind.json
// Golem role: ~/.echoryn/golem.json
// It contains all node-level settings for both Hivemind and Golem roles.
type EchorynConfig struct {
	Version string `json:"version"`

	Node NodeConfig `json:"node"`

	Hivemind HivemindConfig `json:"hivemind,omitempty"`

	Golem GolemConfig `json:"golem,omitempty"`

	Models ModelsConfig `json:"models,omitempty"`

	Plugins PluginsConfig `json:"plugins,omitempty"`

	Diagnostics DiagnosticsConfig `json:"diagnostics,omitempty"`
}

// NodeConfig identifies this node in the cluster.
type NodeConfig struct {
	Role string `json:"role"` // "hivemind" or "golem"
	Name string `json:"name"`
	ID   string `json:"id"`
}

// HivemindConfig holds Hivemind control plane settings.
type HivemindConfig struct {
	Address     string `json:"address"`
	GRPCAddress string `json:"grpc_address"`
	AuthToken   string `json:"auth_token"`
	DataDir     string `json:"data_dir"`
	ConfigPath  string `json:"config_path"`
}

// GolemConfig holds Golem worker node settings.
type GolemConfig struct {
	HivemindAddr string `json:"hivemind_addr,omitempty"`
	JoinToken    string `json:"join_token,omitempty"`
	Workspace    string `json:"workspace"`
	SkillsDir    string `json:"skills_dir"`
	DataDir      string `json:"data_dir"`
}

// ModelsConfig holds LLM provider configuration.
type ModelsConfig struct {
	DefaultProvider string                    `json:"default_provider"`
	DefaultModel    string                    `json:"default_model"`
	Providers       map[string]ProviderConfig `json:"providers,omitempty"`
}

// ProviderConfig holds a single LLM provider's credentials.
type ProviderConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url,omitempty"`
}

// PluginsConfig holds plugin-level settings.
type PluginsConfig struct {
	Enabled bool                   `json:"enabled"`
	Entries map[string]interface{} `json:"entries,omitempty"`
}

// DiagnosticsConfig holds diagnostics/logging settings.
type DiagnosticsConfig struct {
	Telemetry bool   `json:"telemetry"`
	LogLevel  string `json:"log_level"`
}

// DefaultConfig returns a new EchorynConfig with sensible defaults.
func DefaultConfig() *EchorynConfig {
	return &EchorynConfig{
		Version: "1",
		Node: NodeConfig{
			Role: "",
			Name: "",
			ID:   "",
		},
		Models: ModelsConfig{
			Providers: make(map[string]ProviderConfig),
		},
		Plugins: PluginsConfig{
			Enabled: true,
			Entries: make(map[string]interface{}),
		},
		Diagnostics: DiagnosticsConfig{
			Telemetry: false,
			LogLevel:  "info",
		},
	}
}
