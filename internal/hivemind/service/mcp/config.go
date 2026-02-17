package mcp

import (
	"fmt"
	"os"

	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// MCPConfig holds the top-level MCP configuration.
// Compatible with Claude Desktop / VS Code MCP config format.
//
// File format (mcp.json):
//
//	{
//	  "mcpServers": {
//	    "server-name": {
//	      "transport": "stdio",
//	      "command": "npx",
//	      "args": ["-y", "@anthropic/mcp-filesystem-server", "/tmp"]
//	    }
//	  }
//	}
type MCPConfig struct {
	// MCPServers maps server name → server configuration.
	// Uses "mcpServers" key for Claude Desktop compatibility.
	MCPServers map[string]*ServerConfig `json:"mcpServers"`
}

// ServerConfig defines the configuration for a single MCP server.
// Supports two transport types: "stdio" (subprocess) and "sse" (HTTP SSE).
type ServerConfig struct {
	// Transport is the MCP transport protocol: "stdio" or "sse".
	// Default: "stdio".
	Transport string `json:"transport,omitempty"`

	// --- stdio transport fields ---

	// Command is the executable to launch (stdio only).
	// Example: "npx", "python", "/usr/local/bin/mcp-server"
	Command string `json:"command,omitempty"`

	// Args are the command-line arguments (stdio only).
	Args []string `json:"args,omitempty"`

	// Env is the environment variables for the subprocess (stdio only).
	// Format: ["KEY=VALUE", ...].
	Env []string `json:"env,omitempty"`

	// --- sse transport fields ---

	// URL is the SSE endpoint URL (sse only).
	// Example: "http://localhost:8080/sse"
	URL string `json:"url,omitempty"`

	// --- common fields ---

	// ToolFilter is an optional list of tool names to expose.
	// If empty, all tools from the MCP server are exposed.
	ToolFilter []string `json:"toolFilter,omitempty"`
}

// LoadMCPConfig loads the MCP configuration from a JSON file.
// If the file does not exist, returns an empty config (no error).
func LoadMCPConfig(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewMCPConfig(), nil
		}
		return nil, fmt.Errorf("failed to read MCP config file %q: %w", path, err)
	}

	cfg := &MCPConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse MCP config file %q: %w", path, err)
	}

	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]*ServerConfig)
	}

	return cfg, nil
}

// NewMCPConfig creates a default (empty) MCP configuration.
func NewMCPConfig() *MCPConfig {
	return &MCPConfig{
		MCPServers: make(map[string]*ServerConfig),
	}
}

// Validate checks the MCP configuration for obvious errors.
func (c *MCPConfig) Validate() []error {
	var errs []error
	for name, srv := range c.MCPServers {
		if srv.Transport == "" {
			srv.Transport = "stdio"
		}
		switch srv.Transport {
		case "stdio":
			if srv.Command == "" {
				errs = append(errs, fmt.Errorf("mcpServers.%s: command is required for stdio transport", name))
			}
		case "sse":
			if srv.URL == "" {
				errs = append(errs, fmt.Errorf("mcpServers.%s: url is required for sse transport", name))
			}
		default:
			errs = append(errs, fmt.Errorf("mcpServers.%s: unsupported transport %q (must be 'stdio' or 'sse')", name, srv.Transport))
		}
	}
	return errs
}

// Servers returns the server map (convenience accessor).
func (c *MCPConfig) Servers() map[string]*ServerConfig {
	return c.MCPServers
}
