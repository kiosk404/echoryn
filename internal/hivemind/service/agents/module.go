package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/subagent"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/toolloop"
	boltdbStore "github.com/kiosk404/echoryn/internal/hivemind/service/agents/store/boltdb"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/store/inmemory"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm"
	llmEntity "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/mcp"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/subagent/observer"
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

// ensureDefaultAgent checks if the default "main" agent exists in the store;
// if not, it auto-creates one with the system's default model binding.
//
// This is the server-startup counterpart of ChatCompletionsHandler.ensureAgent(),
// which only runs on HTTP requests. Without this bootstrap step, a fresh
// deployment will fail with:
//
//	agent "main" failed to unmarshal agent: agent "main" not found
//
// on any request path that doesn't go through the HTTP handler (e.g., IM gateway,
// echoctl CLI, or plugin-triggered agent runs).
func ensureDefaultAgent(ctx context.Context, agentStore repo.AgentRepository, llmModule *llm.Module, agentID string) {
	_, err := agentStore.Get(ctx, agentID)
	if err == nil {
		// Agent already exists — nothing to do.
		return
	}

	// Resolve default model for binding.
	defaultModel, modelErr := llmModule.Manager.GetDefaultModel(ctx)
	if modelErr != nil {
		logger.Warn("[Agents] cannot auto-create default agent %q: no default model available (check your models config and API keys): %v",
			agentID, modelErr)
		return
	}

	agent := &entity.Agent{
		ID:   agentID,
		Name: agentID,
		ModelRef: llmEntity.ModelRef{
			ProviderID: defaultModel.ProviderID,
			ModelID:    defaultModel.ModelID,
		},
		Fallback: llmEntity.FallbackConfig{
			Primary: llmEntity.ModelRef{
				ProviderID: defaultModel.ProviderID,
				ModelID:    defaultModel.ModelID,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if createErr := agentStore.Create(ctx, agent); createErr != nil {
		logger.Warn("[Agents] failed to auto-create default agent %q: %v", agentID, createErr)
		return
	}
	logger.Info("[Agents] auto-created default agent %q with model %s/%s", agentID, defaultModel.ProviderID, defaultModel.ModelID)
}

// Module is the top-level Agents module, holding all domain services.
//
// It exposes:
//   - Service: Agent CRUD + session management + run execution
//   - Runner: direct access to the AgentRunner for advanced usage
//   - Observer: SubAgent execution observability (metrics, reports, health)
//   - SessionRepo: direct access to session storage for team orchestration
type Module struct {
	Service         service.AgentService
	Runner          *runtime.AgentRunner
	SubAgentManager subagent.Manager
	Observer        observer.Observer
	SessionRepo     repo.SessionRepository // exposed for team orchestration
	boltDB          *boltdbStore.DB        // nil when using inmemory store
}

// Close releases resources held by the module (e.g., BoltDB handle, observer).
func (m *Module) Close() error {
	if m.boltDB != nil {
		return m.boltDB.Close()
	}
	if m.Observer != nil {
		_ = m.Observer.Stop()
	}
	return nil
}

// New creates and initializes the Agents module from a completed config.
func (c CompletedConfig) New(ctx context.Context, deps Dependencies) (*Module, error) {
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
		subAgentStore subagent.Registry
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

	// Bootstrap: ensure the default agent exists in the store.
	// On a fresh deployment the BoltDB is empty — without this step, the first
	// request referencing "main" would fail with "agent not found".
	ensureDefaultAgent(ctx, agentStore, deps.LLM, c.AgentID)

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
	// SubAgent manager (Controller pattern) — now using the independent subagent package.
	obs := observer.New(observer.DefaultConfig())
	if err := obs.Start(); err != nil {
		logger.Warn("[Agents] failed to start SubAgent observer: %v", err)
	}

	// SubAgent manager (Controller pattern) - now using the independent subagent package.
	subAgentMgr := subagent.NewManager(subAgentStore, agentStore, sessionStore, subagent.DefaultConfig(), obs)

	// Break the circular dependency: Runner needs SubAgentManager for
	// Active Run tracking (MarkRunActive/Idle) and announce queue draining.
	// K8S analogy: post-start injection after all controllers are created.
	runner.SetSubAgentManager(subAgentMgr)

	logger.Info("[Agents] Agents module initialized (store=%s, timeout=%s, retries=%d, history_limit=%d, compaction_threshold=%.1f, workspace=%s)",
		c.StoreType, c.RunTimeout, c.MaxRetries, c.MaxHistoryTurns, c.CompactionThreshold, workspaceDir)

	return &Module{
		Service:         svc,
		Runner:          runner,
		SubAgentManager: subAgentMgr,
		Observer:        obs,
		SessionRepo:     sessionStore,
		boltDB:          boltDB,
	}, nil
}
