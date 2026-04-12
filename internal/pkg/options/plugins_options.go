package options

import (
	"fmt"

	"github.com/spf13/pflag"
)

// PluginsOptions holds the top-level configuration for plugin system.
// Aligned with the plugin system configuration file.
type PluginsOptions struct {
	// Enabled controls whether the plugin system is enabled. (default: true)
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// Allow lists plugins that are explicitly allowed to be loaded.
	Allow []string `json:"allow" mapstructure:"allow"`
	// Deny lists plugins that are explicitly denied to be loaded.
	Deny []string `json:"deny" mapstructure:"deny"`
	// Slots controls which plugin occupies each exclusive slot.
	// For exmaple. {"memory": "memory-core"}.
	// Special value "none" disables all plugins of the kind
	Slots PluginSlotsConfig `json:"slots" mapstructure:"slots"`
	// Entries holds per-plugin configuration.
	// Key is the plugin ID. (e.g. "memory-core", "diagnostics", "llm-task")
	Entries map[string]PluginEntryConfig `json:"entries" mapstructure:"entries"`
	// Tools holds tool-level policy configuration (profile, allow/deny, per-provider rules).
	Tools ToolsOptions `json:"tools" mapstructure:"tools"`
}

// ToolsOptions holds tool-level policy configuration.
// Applied to every agent turn via the ToolPolicyPipeline.
type ToolsOptions struct {
	// Profile is the active tool profile preset.
	// Values: "minimal", "coding", "full", "golem", "team"
	// Default: "" (no profile, all tools allowed)
	Profile string `json:"profile" mapstructure:"profile"`

	// Allow lists explicitly allowed tools (supports group:xxx syntax).
	// Empty means allow all (subject to Deny).
	Allow []string `json:"allow" mapstructure:"allow"`

	// Deny lists explicitly denied tools (supports group:xxx syntax).
	// Deny always wins over Allow.
	Deny []string `json:"deny" mapstructure:"deny"`

	// ByProvider holds per-LLM-provider tool allow/deny rules.
	// Key is the provider ID (e.g., "openai", "deepseek", "ollama").
	ByProvider map[string]ToolAllowDeny `json:"by_provider" mapstructure:"by_provider"`
}

// ToolAllowDeny holds allow/deny lists for a specific context.
type ToolAllowDeny struct {
	Allow []string `json:"allow" mapstructure:"allow"`
	Deny  []string `json:"deny" mapstructure:"deny"`
}

// PluginSlotsConfig maps slot kind -> desired Plugin ID
// Aligned with the plugin system configuration file.
type PluginSlotsConfig struct {
	Memory    string `json:"memory" mapstructure:"memory"`
	Channel   string `json:"channel" mapstructure:"channel"`
	Tracing   string `json:"tracing" mapstructure:"tracing"`
	WebSearch string `json:"web-search" mapstructure:"web-search"`
}

// PluginEntryConfig holds per-plugin configuration.
// Aligned with the plugin system configuration file.
type PluginEntryConfig struct {
	Enabled *bool                  `json:"enabled,omitempty" mapstructure:"enabled"`
	Config  map[string]interface{} `json:"config,omitempty" mapstructure:"config"`
}

// NewPluginsOptions returns a new instance of PluginsOptions.
func NewPluginsOptions() *PluginsOptions {
	return &PluginsOptions{
		Enabled: true,
		Allow:   []string{},
		Deny:    []string{},
		Slots: PluginSlotsConfig{
			Memory: "memory-core",
		},
		Entries: make(map[string]PluginEntryConfig),
	}
}

// Validate checks PluginsOptions fields.
func (o *PluginsOptions) Validate() []error {
	var errs []error

	// Validate slot values.
	if o.Slots.Memory != "" && o.Slots.Memory != "none" {
		// Valid plugin IDs are DNS-compatible
		for _, c := range o.Slots.Memory {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
				errs = append(errs, fmt.Errorf("invalid character %q in memory slot name", c))
				break
			}
		}
	}

	return errs
}

// AddFlags adds flags for the plugins options.
// Only global-level switches are exposed as CLI flags.
// Per-plugin configuration is done via the plugin's own configuration file.
func (o *PluginsOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&o.Enabled, "plugins.enabled", o.Enabled, "Enable the plugin system.")
	fs.StringVar(&o.Slots.Memory, "plugins.slots.memory", o.Slots.Memory, "Memory slot name for plugins.")
}
