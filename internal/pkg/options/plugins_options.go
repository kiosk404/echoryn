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
}

// PluginSlotsConfig maps slot kind -> desired Plugin ID
// Aligned with the plugin system configuration file.
type PluginSlotsConfig struct {
	Memory string `json:"memory" mapstructure:"memory"`
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
