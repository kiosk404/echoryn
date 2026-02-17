package mcp

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
)

// Manager manages multiple MCP server connections and providers
// a unified tool discovery interface for the agent execution pipeline.
type Manager interface {
	// Initialize connects to all configured MCP servers.
	Initialize(ctx context.Context) error

	// GetAllTools returns a list of all available tools from all connected servers.
	GetAllTools() []tool.BaseTool

	// GetToolsByServer returns a list of tools available from a specific server.
	GetToolsByServer(serverName string) []tool.BaseTool

	// Reconnect closes the current connection and establishes a new one.
	Reconnect(ctx context.Context, serverName string) error

	// ServerNames returns a list of all configured server names.
	ServerNames() []string

	// ServerStatus returns the current status of a specific server.
	ServerStatus(serverName string) ServerStatus

	// Close closes all MCP server connections.
	Close() error
}
