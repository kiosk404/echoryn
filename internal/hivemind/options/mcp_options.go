package options

import (
	"errors"

	"github.com/spf13/pflag"
)

// MCPOptions holds options for the MCP (Model Context Protocol) subsystem.
// MCP uses a standalone configuration file
type MCPOptions struct {
	// ConfigFile is the path to the MCP configuration file.
	// Default: "mcp.json".
	ConfigFile string `json:"config_file" mapstructure:"config_file"`
}

// NewMCPOptions creates a default MCPOptions instance.
func NewMCPOptions() *MCPOptions {
	return &MCPOptions{
		ConfigFile: "conf/mcp.json",
	}
}

// Validate checks the MCPOptions for correctness.
func (o *MCPOptions) Validate() error {
	if o.ConfigFile == "" {
		return errors.New("config_file is required")
	}
	return nil
}

// AddFlags adds the MCPOptions flags to the given flag set.
func (o *MCPOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ConfigFile, "mcp.config-file", o.ConfigFile, "Path to the MCP configuration file.")
}
