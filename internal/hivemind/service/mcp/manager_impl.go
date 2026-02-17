package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// managerImpl is the default implementation of Manager.
type managerImpl struct {
	mu      sync.RWMutex
	servers map[string]*MCPServer
	order   []string // preserves config order
}

// Ensure managerImpl implements Manager.
var _ Manager = (*managerImpl)(nil)

func newManager(cfg *MCPConfig) *managerImpl {
	m := &managerImpl{
		servers: make(map[string]*MCPServer, len(cfg.MCPServers)),
		order:   make([]string, 0, len(cfg.MCPServers)),
	}

	for name, srvCfg := range cfg.MCPServers {
		m.servers[name] = NewMCPServer(name, srvCfg)
		m.order = append(m.order, name)
	}

	return m
}

// Initialize connects to all configured MCP servers concurrently.
// Individual server failures are logged but don't prevent other servers from connecting.
func (m *managerImpl) Initialize(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.servers) == 0 {
		logger.Info("[MCP] no MCP servers configured, skipping initialization")
		return nil
	}

	logger.Info("[MCP] initializing %d MCP servers...", len(m.servers))

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var errs []error

	for _, srv := range m.servers {
		wg.Add(1)
		go func(s *MCPServer) {
			defer wg.Done()
			if err := s.Connect(ctx); err != nil {
				errMu.Lock()
				errs = append(errs, err)
				errMu.Unlock()
				logger.Warn("[MCP] server %q failed to connect: %v", s.Name(), err)
			}
		}(srv)
	}

	wg.Wait()

	// Count successes.
	connected := 0
	for _, srv := range m.servers {
		if srv.Status() == ServerStatusConnected {
			connected++
		}
	}

	logger.Info("[MCP] initialization complete: %d/%d servers connected", connected, len(m.servers))

	if len(errs) > 0 && connected == 0 {
		return fmt.Errorf("[MCP] all servers failed to connect (%d errors)", len(errs))
	}

	return nil
}

// GetAllTools aggregates tools from all connected MCP servers.
func (m *managerImpl) GetAllTools() []tool.BaseTool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []tool.BaseTool
	for _, name := range m.order {
		srv := m.servers[name]
		if srv.Status() == ServerStatusConnected {
			all = append(all, srv.Tools()...)
		}
	}
	return all
}

// GetToolsByServer returns tools from a specific server.
func (m *managerImpl) GetToolsByServer(serverName string) []tool.BaseTool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	srv, ok := m.servers[serverName]
	if !ok {
		return nil
	}
	return srv.Tools()
}

// Reconnect re-establishes the connection to a specific server.
func (m *managerImpl) Reconnect(ctx context.Context, serverName string) error {
	m.mu.RLock()
	srv, ok := m.servers[serverName]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("[MCP] server %q not found", serverName)
	}

	return srv.Reconnect(ctx)
}

// ServerNames returns the names of all configured servers in config order.
func (m *managerImpl) ServerNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]string, len(m.order))
	copy(result, m.order)
	return result
}

// ServerStatus returns the status of a specific server.
func (m *managerImpl) ServerStatus(serverName string) ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	srv, ok := m.servers[serverName]
	if !ok {
		return ServerStatusDisconnected
	}
	return srv.Status()
}

// Close closes all MCP server connections.
func (m *managerImpl) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, srv := range m.servers {
		srv.Close()
	}

	logger.Info("[MCP] all servers closed")
	return nil
}
