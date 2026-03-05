package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/toolloop"
	boltdbStore "github.com/kiosk404/echoryn/internal/hivemind/service/agents/store/boltdb"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/store/inmemory"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm"
	"github.com/kiosk404/echoryn/internal/hivemind/service/mcp"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/paths"
)

// Config holds the configuration for the Agents module.
// Follows K8S-style: Config → Complete() → New(ctx, deps).
//
// Path resolution follows the ~/.echoryn state directory convention.
// BoltDBPath and WorkspaceDir are derived from paths.Resolve* when empty.
type Config struct {
	// RunTimeout is the maximum duration for a single run (default: 5m).
	RunTimeout time.Duration `json:"run_timeout,omitempty"`

	// MaxRetries is the maximum retry attempts on transient failures (default: 3).
	MaxRetries int `json:"max_retries,omitempty"`

	// LoopDetection configures the tool call loop detection (circuit-breaker).
	// Aligned with OpenClaw's tool-loop-detection.ts.
	// Zero-value means use defaults (enabled, thresholds, warn=10, critical=20, breaker=30).
	LoopDetection *LoopDetectionConfig `json:"loop_detection,omitempty"`

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

	// --- Storage ---

	// StoreType selects the persistence backend: "inmemory" or "boltdb".
	// Default: "boltdb".
	StoreType string `json:"store_type,omitempty"`

	// AgentID identifies which agent's data directories to use.
	// Default: "main".
	AgentID string `json:"agent_id,omitempty"`
}

// CompletedConfig is the validated and completed configuration.
type CompletedConfig struct {
	*Config
}

// LoopDetectionConfig exposes the tool loop detection thresholds for JSON config.
// Aligned with OpenClaw's ToolLoopDetectionConfig.
type LoopDetectionConfig struct {
	Enabled                       *bool `json:"enabled,omitempty"`
	HistorySize                   int   `json:"history_size,omitempty"`
	WarningThreshold              int   `json:"warning_threshold,omitempty"`
	CriticalThreshold             int   `json:"critical_threshold,omitempty"`
	GlobalCircuitBreakerThreshold int   `json:"global_circuit_breaker_threshold,omitempty"`
}

// toToolloopConfig converts the JSON config to the internal toolloop.Config.
func (c *LoopDetectionConfig) toToolloopConfig() toolloop.Config {
	if c == nil {
		return toolloop.DefaultConfig()
	}
	cfg := toolloop.DefaultConfig()
	if c.Enabled != nil {
		cfg.Enabled = *c.Enabled
	}
	if c.HistorySize > 0 {
		cfg.HistorySize = c.HistorySize
	}
	if c.WarningThreshold > 0 {
		cfg.WarningThreshold = c.WarningThreshold
	}
	if c.CriticalThreshold > 0 {
		cfg.CriticalThreshold = c.CriticalThreshold
	}
	if c.GlobalCircuitBreakerThreshold > 0 {
		cfg.GlobalCircuitBreakerThreshold = c.GlobalCircuitBreakerThreshold
	}
	return cfg
}

// Complete validates and fills defaults.
func (c *Config) Complete() CompletedConfig {
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
		c.StoreType = "boltdb"
	}
	if c.AgentID == "" {
		c.AgentID = paths.DefaultAgentID()
	}
	return CompletedConfig{c}
}

// Dependencies holds the external modules required by the Agents module.
type Dependencies struct {
	LLM     *llm.Module
	Plugins *plugin.Framework
	MCP     mcp.Manager // MCP tool provider (maybe nil if no MCP servers configured)
}

// Module is the top-level Agents module, holding all domain services.
//
// It exposes:
//   - Service: Agent CRUD + session management + run execution
//   - Runner: direct access to the AgentRunner for advanced usage
type Module struct {
	Service         service.AgentService
	Runner          *runtime.AgentRunner
	SubAgentManager service.SubAgentManager
	boltDB          *boltdbStore.DB // nil when using inmemory store
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
		return nil, fmt.Errorf("plugin framework dependency is required")
	}

	// Resolve paths from the centralized ~/.echoryn state directory
	boltDBPath := paths.ResolveSessionStorePath(c.AgentID)
	workspaceDir := paths.ResolveWorkspaceDir(c.AgentID, "")

	// Infrastructure layer: select store backend.
	var (
		agentStore    repo.AgentRepository
		sessionStore  repo.SessionRepository
		runStore      repo.RunRepository
		subAgentStore runtime.SubAgentRegistry
		boltDB        *boltdbStore.DB
	)

	switch c.StoreType {
	case "boltdb":
		var err error
		boltDB, err = boltdbStore.Open(boltDBPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open boltdb at %s: %w", boltDBPath, err)
		}
		agentStore = boltdbStore.NewAgentStore(boltDB)
		sessionStore = boltdbStore.NewSessionStore(boltDB)
		runStore = boltdbStore.NewRunStore(boltDB)
		subAgentStore = boltdbStore.NewSubAgentStore(boltDB)
		logger.Info("[Agents] using BoltDB store at %s", boltDBPath)
	default:
		agentStore = inmemory.NewAgentStore()
		sessionStore = inmemory.NewSessionStore()
		runStore = inmemory.NewRunStore()
		subAgentStore = inmemory.NewSubAgentStore()
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
			RunTimeout:          c.RunTimeout,
			MaxRetries:          c.MaxRetries,
			MaxHistoryTurns:     c.MaxHistoryTurns,
			CompactionThreshold: c.CompactionThreshold,
			KeepRecentTurns:     c.KeepRecentTurns,
			LoopDetection:       c.LoopDetection.toToolloopConfig(),
			WorkspaceDir:        workspaceDir,
		},
	)

	// Application service layer.
	svc := service.NewAgentService(agentStore, sessionStore, runStore, runner)
	// SubAgent manager (Controller pattern)
	subAgentMgr := runtime.NewSubAgentManager(subAgentStore, agentStore, sessionStore, runner, runtime.DefaultSubAgentManagerConfig())

	logger.Info("[Agents] Agents module initialized (store=%s, timeout=%s, retries=%d, history_limit=%d, compaction_threshold=%.1f, workspace=%s)",
		c.StoreType, c.RunTimeout, c.MaxRetries, c.MaxHistoryTurns, c.CompactionThreshold, workspaceDir)

	return &Module{
		Service:         svc,
		Runner:          runner,
		SubAgentManager: subAgentMgr,
		boltDB:          boltDB,
	}, nil
}
