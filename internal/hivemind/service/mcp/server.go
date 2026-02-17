package mcp

import (
	"context"
	"fmt"
	"sync"

	mcpTool "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// ServerStatus represents the connection state of an MCP server.
type ServerStatus int

const (
	ServerStatusDisconnected ServerStatus = iota
	ServerStatusConnecting
	ServerStatusConnected
	ServerStatusError
)

func (s ServerStatus) String() string {
	switch s {
	case ServerStatusDisconnected:
		return "Disconnected"
	case ServerStatusConnecting:
		return "Connecting"
	case ServerStatusConnected:
		return "Connected"
	case ServerStatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// MCPServer represents an MCP server instance.
type MCPServer struct {
	name   string
	config *ServerConfig

	mu     sync.RWMutex
	client client.MCPClient
	tools  []tool.BaseTool
	status ServerStatus
	err    error
}

// NewMCPServer creates a new MCP server instance.
func NewMCPServer(name string, cfg *ServerConfig) *MCPServer {

	return &MCPServer{
		name:   name,
		status: ServerStatusDisconnected,
		config: cfg,
	}
}

// Name returns the server name
func (s *MCPServer) Name() string {
	return s.name
}

// Status returns the current connection status.
func (s *MCPServer) Status() ServerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// Tools returns the discovered tools (empty if not connected).
func (s *MCPServer) Tools() []tool.BaseTool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]tool.BaseTool, len(s.tools))
	copy(result, s.tools)
	return result
}

// Connect establishes a connection to the MCP server and discovers tools
func (s *MCPServer) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = ServerStatusConnecting
	s.err = nil

	// Create transport-specific client.
	cli, err := s.createClient()
	if err != nil {
		s.status = ServerStatusError
		s.err = err
		return fmt.Errorf("[MCP] server %q: failed to create client: %w", s.name, err)
	}

	// Initialize the MCP protocol handshake.
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "Echoryn-hivemind",
		Version: "0.0.1",
	}

	if _, err := cli.Initialize(ctx, initReq); err != nil {
		s.status = ServerStatusError
		s.err = err
		return fmt.Errorf("[MCP] server %q: failed to initialize: %w", s.name, err)
	}

	// Discover tools via eino-ext/mcp.GetTools
	tools, err := mcpTool.GetTools(ctx, &mcpTool.Config{
		Cli:          cli,
		ToolNameList: s.config.ToolFilter,
	})
	if err != nil {
		s.status = ServerStatusError
		s.err = err
		return fmt.Errorf("[MCP] server %q: failed to get tools: %w", s.name, err)
	}

	s.client = cli
	s.tools = tools
	s.status = ServerStatusConnected

	return nil
}

// Reconnect closes the current connection and establishes a new one.
func (s *MCPServer) Reconnect(ctx context.Context) error {
	s.Close()
	return s.Connect(ctx)
}

// Close closes the current connection and releases resources.
func (s *MCPServer) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		if err := s.client.Close(); err != nil {
			logger.Warn("[MCP] server %q: failed to close client: %v", s.name, err)
		}
		s.client = nil
	}

	s.tools = nil
	s.status = ServerStatusDisconnected
	s.err = nil
}

// createClient creates a transport-specific MCP client.
// Must be called with s.mu held.
func (s *MCPServer) createClient() (client.MCPClient, error) {
	switch s.config.Transport {
	case "stdio":
		return client.NewStdioMCPClient(s.config.Command, s.config.Env, s.config.Args...)
	case "sse":
		return client.NewSSEMCPClient(s.config.URL)
	default:
		return nil, fmt.Errorf("unknown transport: %s", s.config.Transport)
	}
}
