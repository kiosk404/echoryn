package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime"
	boltdbStore "github.com/kiosk404/echoryn/internal/hivemind/service/agents/store/boltdb"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/store/inmemory"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm"
	"github.com/kiosk404/echoryn/internal/hivemind/service/mcp"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// Config holds the configuration for the Agents module.
// Follows K8S-style: Config → Complete() → New(ctx, deps).
type Config struct {
	// DefaultMaxTurns is the maximum tool-call turns per run (default: 10).
	DefaultMaxTurns int `json:"default_max_turns,omitempty"`

	// RunTimeout is the maximum duration for a single run (default: 5m).
	RunTimeout time.Duration `json:"run_timeout,omitempty"`

	// MaxRetries is the maximum retry attempts on transient failures (default: 3).
	MaxRetries int `json:"max_retries,omitempty"`

	// --- Context management (Phase 2A) ---

	// MaxHistoryTurns limits how many recent user turns of history to load.
	// 0 means no limit (load all active messages).
	// Default: 50 (matches OpenClaw's default DM history limit).
	MaxHistoryTurns int `json:"max_history_turns,omitempty"`

	// CompactionThreshold: when (estimated tokens / window) > this, trigger compaction.
	// Default: 0.8.
	CompactionThreshold float64 `json:"compaction_threshold,omitempty"`

	// KeepRecentTurns: number of recent user→assistant turn pairs to preserve during compaction.
	// Default: 3.
	KeepRecentTurns int `json:"keep_recent_turns,omitempty"`

	// --- Storage (P0) ---

	// StoreType selects the persistence backend: "inmemory" or "boltdb".
	// Default: "inmemory".
	StoreType string `json:"store_type,omitempty"`

	// BoltDBPath is the file path for BoltDB storage (when StoreType="boltdb").
	// Default: "data/eidolon.db".
	BoltDBPath string `json:"boltdb_path,omitempty"`
}

// CompletedConfig is the validated and completed configuration.
type CompletedConfig struct {
	*Config
}

// Complete validates and fills defaults.
func (c *Config) Complete() CompletedConfig {
	if c.DefaultMaxTurns <= 0 {
		c.DefaultMaxTurns = 10
	}
	if c.RunTimeout <= 0 {
		c.RunTimeout = 5 * time.Minute
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
	if c.MaxHistoryTurns <= 0 {
		c.MaxHistoryTurns = 50
	}
	if c.CompactionThreshold <= 0 {
		c.CompactionThreshold = 0.8
	}
	if c.KeepRecentTurns <= 0 {
		c.KeepRecentTurns = 3
	}
	if c.StoreType == "" {
		c.StoreType = "inmemory"
	}
	if c.BoltDBPath == "" {
		c.BoltDBPath = "data/eidolon.db"
	}
	return CompletedConfig{c}
}

// Dependencies holds the external modules required by the Agents module.
type Dependencies struct {
	LLM     *llm.Module
	Plugins *plugin.Framework
	MCP     mcp.Manager // MCP tool provider (may be nil if no MCP servers configured)
}

// Module is the top-level Agents module, holding all domain services.
//
// It exposes:
//   - Service: Agent CRUD + session management + run execution
//   - Runner: direct access to the AgentRunner for advanced usage
type Module struct {
	Service service.AgentService
	Runner  *runtime.AgentRunner
	boltDB  *boltdbStore.DB // nil when using inmemory store
}

// Close releases resources held by the module (e.g., BoltDB handle).
func (m *Module) Close() error {
	if m.boltDB != nil {
		return m.boltDB.Close()
	}
	return nil
}

// New creates and initializes the Agents module from a completed config.
func (c CompletedConfig) New(_ context.Context, deps Dependencies) (*Module, error) {
	logger.Info("[Agents] creating Agents module...")

	if deps.LLM == nil {
		return nil, fmt.Errorf("LLM module dependency is required")
	}
	if deps.Plugins == nil {
		return nil, fmt.Errorf("Plugin framework dependency is required")
	}

	// Infrastructure layer: select store backend.
	var (
		agentStore   repo.AgentRepository
		sessionStore repo.SessionRepository
		runStore     repo.RunRepository
		boltDB       *boltdbStore.DB
	)

	switch c.StoreType {
	case "boltdb":
		var err error
		boltDB, err = boltdbStore.Open(c.BoltDBPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open boltdb at %s: %w", c.BoltDBPath, err)
		}
		agentStore = boltdbStore.NewAgentStore(boltDB)
		sessionStore = boltdbStore.NewSessionStore(boltDB)
		runStore = boltdbStore.NewRunStore(boltDB)
		logger.Info("[Agents] using BoltDB store at %s", c.BoltDBPath)
	default:
		agentStore = inmemory.NewAgentStore()
		sessionStore = inmemory.NewSessionStore()
		runStore = inmemory.NewRunStore()
		logger.Info("[Agents] using in-memory store")
	}

	// Runtime: AgentRunner with all dependencies.
	runner := runtime.NewAgentRunner(
		agentStore,
		sessionStore,
		runStore,
		deps.LLM,
		deps.Plugins,
		deps.MCP,
		runtime.AgentRunnerConfig{
			DefaultMaxTurns:     c.DefaultMaxTurns,
			RunTimeout:          c.RunTimeout,
			MaxRetries:          c.MaxRetries,
			MaxHistoryTurns:     c.MaxHistoryTurns,
			CompactionThreshold: c.CompactionThreshold,
			KeepRecentTurns:     c.KeepRecentTurns,
		},
	)

	// Application service layer.
	svc := service.NewAgentService(agentStore, sessionStore, runStore, runner)

	logger.Info("[Agents] Agents module initialized (store=%s, max_turns=%d, timeout=%s, retries=%d, history_limit=%d, compaction_threshold=%.1f)",
		c.StoreType, c.DefaultMaxTurns, c.RunTimeout, c.MaxRetries, c.MaxHistoryTurns, c.CompactionThreshold)

	return &Module{
		Service: svc,
		Runner:  runner,
		boltDB:  boltDB,
	}, nil
}
