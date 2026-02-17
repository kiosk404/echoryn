package mcp

import (
	"context"

	"github.com/kiosk404/echoryn/pkg/logger"
)

type Config struct {
	MCPConfig *MCPConfig
}

// CompletedConfig is the completed configuration for MCP.
type CompletedConfig struct {
	*Config
}

// Complete validates and fills defaults.
func (c *Config) Complete() CompletedConfig {
	if c.MCPConfig == nil {
		c.MCPConfig = NewMCPConfig()
	}

	// Fill default values.
	for _, srv := range c.MCPConfig.MCPServers {
		if srv.Transport == "" {
			srv.Transport = "stdio"
		}
	}

	return CompletedConfig{c}
}

// Module is the top-level MCP module.
type Module struct {
	Manager Manager
}

// New creates and initializes the MCP module.
func (c CompletedConfig) New(ctx context.Context) (*Module, error) {
	mgr := newManager(c.MCPConfig)

	if err := mgr.Initialize(ctx); err != nil {
		logger.Warn("[MCP] initialization had error: %v", err)
	}
	logger.Info("[MCP] module initialized (%d servers configured)", len(c.MCPConfig.MCPServers))
	return &Module{Manager: mgr}, nil
}

// Close releases all resources held by the MCP module.
func (m *Module) Close() error {
	if m.Manager != nil {
		return m.Manager.Close()
	}
	return nil
}
