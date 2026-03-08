package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/agentflow"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/toolloop"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm"
	"github.com/kiosk404/echoryn/internal/hivemind/service/mcp"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/utils/safego"
)

// RunRequest is the input to AgentRunner.Run.
type RunRequest struct {
	// AgentID is the agent to execute.
	AgentID string

	// SessionID is the session to use (optional; creates new if empty).
	SessionID string

	// Input is the user message text.
	Input string
}

// AgentRunner is the top-level orchestrator for agent execution.
//
// This is the Echoryn equivalent of:
//   - airi-go: agent_run_impl.go + singleagent_run.go
//   - OpenClaw: agent-runner.ts (7-layer pipeline)
//
// Execution flow:
//  1. Resolve Agent from repo
//  2. Load/create Session
//  3. Create Run record + state machine
//  4. Fire HookBeforeAgentStart (plugin hooks, e.g., memory injection)
//  5. Resolve context window (ContextWindowGuard)
//  6. Build LLM context (system prompt + compaction summary + memory + history + input)
//  7. Apply context pruning (soft-trim / hard-clear)
//  8. Create schema.Pipe[AgentEvent] for streaming
//  9. Launch async goroutine → TurnExecutor.Execute → Eino graph.Stream
//  10. Post-turn: proactive compaction check
//  11. Return StreamReader immediately for client consumption
type AgentRunner struct {
	agentRepo       repo.AgentRepository
	sessionRepo     repo.SessionRepository
	runRepo         repo.RunRepository
	llmModule       *llm.Module
	pluginFramework *plugin.Framework
	mcpManager      mcp.Manager
	turnExecutor    *TurnExecutor
	contextBuilder  *ContextBuilder
	windowGuard     *ContextWindowGuard
	compactor       *Compactor
	loopDetectCfg   toolloop.Config
	runTimeout      time.Duration

	// activeRuns tracks in-flight abort controllers by run ID.
	// This enables external callers to cancel a running execution via Abort().
	// Pattern borrowed from subAgentManagerImpl.abortFuncs.
	mu         sync.Mutex
	activeRuns map[string]*AbortController
}

// AgentRunnerConfig holds configuration for the AgentRunner.
type AgentRunnerConfig struct {
	RunTimeout          time.Duration
	MaxRetries          int
	MaxHistoryTurns     int
	CompactionThreshold float64
	KeepRecentTurns     int
	// WorkspaceDir is the resolved workspace directory (e.g. ~/.echoryn/workspace).
	// Convention prompt files (SOUL.md, IDENTITY.md, AGENTS.md, prompts/*.md)
	// are read directly from this directory.
	LoopDetection toolloop.Config
	// WorkspaceDir is the resolved workspace directory (e.g. ~/.echoryn/workspace)
	// Convention prompt files (SOUL.md, IDENTITY.md, AGENTS.md, prompts/*.md)
	// are read directly from this directory.
	WorkspaceDir string
}

// NewAgentRunner creates a new AgentRunner with all dependencies.
func NewAgentRunner(
	agentRepo repo.AgentRepository,
	sessionRepo repo.SessionRepository,
	runRepo repo.RunRepository,
	llmModule *llm.Module,
	pluginFramework *plugin.Framework,
	mcpManager mcp.Manager,
	cfg AgentRunnerConfig,
) *AgentRunner {
	if cfg.RunTimeout <= 0 {
		cfg.RunTimeout = 5 * time.Minute
	}

	// Initialize loop detection config
	loopCfg := cfg.LoopDetection
	if !loopCfg.Enabled && loopCfg.GlobalCircuitBreakerThreshold == 0 {
		// Zero-value config: use defaults.
		loopCfg = toolloop.DefaultConfig()
	}

	estimator := NewTokenEstimator(DefaultCharsPerToken)
	pruner := NewContextPruner(estimator, DefaultPrunerConfig())
	contextBuilder := NewContextBuilder(estimator, pruner, cfg.MaxHistoryTurns)

	// Wire up PromptPipeline: use plugin framework's pipeline if available,
	// otherwise create a default one with builtin sections.
	var pipeline *prompt.Pipeline
	if pluginFramework != nil && pluginFramework.PromptPipeline() != nil {
		pipeline = pluginFramework.PromptPipeline()
	} else {
		pipeline = prompt.NewDefaultPipeline()
	}
	contextBuilder.SetPipeline(pipeline)

	// Wire up WorkspaceLoader: watch workspace directory for prompt files.
	if cfg.WorkspaceDir != "" {
		if wl := prompt.NewWorkspaceLoader(cfg.WorkspaceDir); wl != nil {
			pipeline.SetWorkspaceLoader(wl)
			logger.Info("[AgentRunner] WorkspaceLoader attached for %s", cfg.WorkspaceDir)
		}
	}

	var windowGuard *ContextWindowGuard
	if llmModule != nil {
		windowGuard = NewContextWindowGuard(llmModule.Manager, DefaultContextWindow)
	}

	compactorCfg := DefaultCompactorConfig()
	if cfg.CompactionThreshold > 0 {
		compactorCfg.CompactionThreshold = cfg.CompactionThreshold
	}
	if cfg.KeepRecentTurns > 0 {
		compactorCfg.KeepRecentTurns = cfg.KeepRecentTurns
	}
	compactor := NewCompactor(estimator, compactorCfg, pluginFramework.Registry())

	flowBuilder := agentflow.NewAgentFlowBuilder()
	turnExecutor := NewTurnExecutor(flowBuilder, llmModule.Fallback, contextBuilder, cfg.MaxRetries)

	return &AgentRunner{
		agentRepo:       agentRepo,
		sessionRepo:     sessionRepo,
		runRepo:         runRepo,
		llmModule:       llmModule,
		pluginFramework: pluginFramework,
		mcpManager:      mcpManager,
		turnExecutor:    turnExecutor,
		contextBuilder:  contextBuilder,
		windowGuard:     windowGuard,
		compactor:       compactor,
		loopDetectCfg:   loopCfg,
		runTimeout:      cfg.RunTimeout,
	}
}

// Run executes an agent interaction and returns a streaming event reader.
//
// Callers consume events via sr.Recv() until io.EOF.
func (r *AgentRunner) Run(ctx context.Context, req *RunRequest) (*schema.StreamReader[*entity.AgentEvent], error) {
	// 1. Resolve agent.
	agent, err := r.agentRepo.Get(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agent %q: %w", req.AgentID, err)
	}

	// 2. Load or create session.
	session, err := r.resolveSession(ctx, agent, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("session resolution failed: %w", err)
	}

	// 3. Create run record.
	run := &entity.Run{
		ID:        uuid.New().String(),
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    entity.RunStatusCreated,
		Input:     req.Input,
		CreatedAt: time.Now(),
	}
	if err := r.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}

	// 4. Create state machine.
	stateMachine := NewRunStateMachine(run, r.runRepo)
	if err := stateMachine.TransitionToInProgress(); err != nil {
		return nil, err
	}

	// 5. Create abort controller.
	abort := NewAbortController(ctx, run.ID, r.runTimeout)

	// 6. Create streaming event pipe (airi-go schema.Pipe pattern).
	sr, sw := schema.Pipe[*entity.AgentEvent](20)

	// 7. Launch async execution.
	safego.Go(abort.Context(), func() {
		defer abort.CleanUp()
		defer sw.Close()

		r.executeRun(abort.Context(), agent, session, run, stateMachine, sw, abort, req.Input)
	})

	// 8. Emit initial run status event.
	sw.Send(&entity.AgentEvent{
		Type:      entity.EventRunStatus,
		RunStatus: entity.RunStatusInProgress,
	}, nil)

	return sr, nil
}

// executeRun is the async execution body running inside safego.Go.
func (r *AgentRunner) executeRun(
	ctx context.Context,
	agent *entity.Agent,
	session *entity.Session,
	run *entity.Run,
	stateMachine *RunStateMachine,
	sw *schema.StreamWriter[*entity.AgentEvent],
	abort *AbortController,
	userInput string,
) {
	// Fire before_agent_start hook (memory injection, etc.).
	injectedMessages := r.fireBeforeAgentStart(ctx, agent, session)

	// Resolve context window.
	windowInfo := r.resolveWindowInfo(ctx, agent)

	// Adapt plugin tools to Eino tools.
	pluginTools := agentflow.AdaptPluginTools(r.pluginFramework.Registry(), agent.Tools)

	// Merge MCP tools, filtered by agent.MCPServers (empty = all servers).
	tools := pluginTools
	var mcpToolsList []tool.BaseTool
	if r.mcpManager != nil {
		if len(agent.MCPServers) == 0 {
			mcpToolsList = r.mcpManager.GetAllTools()
		} else {
			for _, name := range agent.MCPServers {
				mcpToolsList = append(mcpToolsList, r.mcpManager.GetToolsByServer(name)...)
			}
		}
		if len(mcpToolsList) > 0 {
			tools = append(tools, mcpToolsList...)
			logger.DebugX(pkg.ModuleName, "[AgentRunner] merged %d plugin tools + %d MCP tools", len(pluginTools), len(mcpToolsList))
		}
	}

	// Build PromptContext with tool summaries for the PromptPipeline.
	promptCtx := r.buildPromptContext(agent, session, tools)
	logger.InfoX(pkg.ModuleName, "[AgentRunner] promptCtx built: mode=%s tools=%d clusterInfo=%v agent=%v",
		promptCtx.Mode, len(promptCtx.Tools), promptCtx.ClusterInfo != nil, promptCtx.Agent != nil)

	// Build LLM context with pruning.
	buildResult := r.contextBuilder.Build(agent, session, userInput, injectedMessages, windowInfo, promptCtx)
	messages := buildResult.Messages

	logger.DebugX(pkg.ModuleName, "[AgentRunner] context built: %d messages, ~%d tokens, window=%d usable=%d",
		len(messages), buildResult.EstimatedTokens, windowInfo.WindowSize, windowInfo.UsableTokens)

	// Create per-run loop detector (OpenClaw's tool-loop-detection.ts.)
	// Each run gets its own detector with a fresh sliding window.
	loopDetector := toolloop.NewDetector(r.loopDetectCfg)

	// Execute the turn.
	result, err := r.turnExecutor.Execute(ctx, &TurnRequest{
		Agent:        agent,
		Messages:     messages,
		Tools:        tools,
		LoopDetector: loopDetector,
		EventWriter:  sw,
		Session:      session,
		WindowInfo:   windowInfo,
		Compactor:    r.compactor,
	}, abort)

	if err != nil {
		logger.Warn("[AgentRunner] run %s failed: %v", run.ID, err)
		stateMachine.TransitionToFailed("execution_error", err.Error())

		sw.Send(&entity.AgentEvent{
			Type:      entity.EventRunStatus,
			RunStatus: entity.RunStatusFailed,
			Error:     err.Error(),
		}, nil)

		r.fireAgentEnd(ctx, agent, session, run)
		_ = r.runRepo.Update(ctx, run)
		return
	}

	// Success: extract final message.
	finalContent := ""
	if result.FinalMessage != nil {
		finalContent = result.FinalMessage.Content
	}

	stateMachine.TransitionToCompleted(finalContent, result.Usage)
	run.ModelRef = result.ModelRef.String()

	// Persist: update session history.
	session.AppendMessage(entity.NewUserMessage(userInput))
	session.AppendMessage(entity.NewAssistantMessage(finalContent))
	session.AddUsage(result.Usage)
	_ = r.sessionRepo.Update(ctx, session)

	// Persist: update run.
	_ = r.runRepo.Update(ctx, run)

	// Proactive compaction check (OpenClaw equivalent: post-turn threshold maintenance).
	r.checkProactiveCompaction(ctx, agent, session, windowInfo, sw)

	// Emit done event.
	sw.Send(&entity.AgentEvent{
		Type:      entity.EventDone,
		RunStatus: entity.RunStatusCompleted,
		Usage:     result.Usage,
	}, nil)

	// Fire agent_end hook.
	r.fireAgentEnd(ctx, agent, session, run)

	logger.InfoX(pkg.ModuleName, "[AgentRunner] run %s completed (model=%s)", run.ID, run.ModelRef)
}

// resolveWindowInfo resolves context window using the guard, or returns defaults.
func (r *AgentRunner) resolveWindowInfo(ctx context.Context, agent *entity.Agent) ContextWindowInfo {
	if r.windowGuard != nil {
		return r.windowGuard.Resolve(ctx, agent.ModelRef)
	}
	return ContextWindowInfo{
		WindowSize:    DefaultContextWindow,
		ReserveTokens: 4096,
		UsableTokens:  DefaultContextWindow - 4096,
	}
}

// checkProactiveCompaction checks if the session needs compaction after a successful turn
// and performs it if necessary (OpenClaw's post-turn threshold maintenance).
func (r *AgentRunner) checkProactiveCompaction(
	ctx context.Context,
	agent *entity.Agent,
	session *entity.Session,
	windowInfo ContextWindowInfo,
	sw *schema.StreamWriter[*entity.AgentEvent],
) {
	if r.compactor == nil {
		return
	}

	if !r.compactor.ShouldCompact(session, windowInfo) {
		return
	}

	logger.InfoX(pkg.ModuleName, "[AgentRunner] proactive compaction triggered for session %s", session.ID)

	params := agent.LLMParams()
	compactModel, _, err := r.llmModule.Fallback.GetChatModelWithFallback(
		ctx, agent.Fallback, params)
	if err != nil {
		logger.WarnX(pkg.ModuleName, "[AgentRunner] proactive compaction skipped: no model available: %v", err)
		return
	}

	_, err = r.compactor.Compact(ctx, session, compactModel, windowInfo, agent)
	if err != nil {
		logger.WarnX(pkg.ModuleName, "[AgentRunner] proactive compaction failed: %v", err)
		return
	}

	_ = r.sessionRepo.Update(ctx, session)

	logger.InfoX(pkg.ModuleName, "[AgentRunner] proactive compaction completed for session %s (count=%d)",
		session.ID, session.CompactionCount)
}

// resolveSession loads an existing session or creates a new one.
// If sessionID is provided but not found, the new session reuses that ID
// so that subsequent calls with the same sesionID find the same session
// (e.g. IM channel conversations using "feishu:{chatID}" as a stable key)
func (r *AgentRunner) resolveSession(ctx context.Context, agent *entity.Agent, sessionID string) (*entity.Session, error) {
	if sessionID != "" {
		session, err := r.sessionRepo.Get(ctx, sessionID)
		if err != nil && !errors.Is(err, errno.ErrSessionNotFound) {
			return nil, err
		}
		if session != nil {
			return session, nil
		}
	}

	// Use the calller-provided sessionID when available so the mapping is stable.
	// fall back a random UUID for callers that don't specify one.
	id := sessionID
	if id == "" {
		id = uuid.New().String()
	}

	session := &entity.Session{
		ID:        id,
		AgentID:   agent.ID,
		Messages:  make([]*entity.Message, 0),
		Metadata:  make(map[string]string),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := r.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// fireBeforeAgentStart fires the before_agent_start hook and collects injected messages.
func (r *AgentRunner) fireBeforeAgentStart(ctx context.Context, agent *entity.Agent, session *entity.Session) []*entity.Message {
	hookData := map[string]interface{}{
		"agent":   agent,
		"session": session,
	}
	if err := plugin.FireHooks(ctx, r.pluginFramework.Registry(), plugin.HookBeforeAgentStart, hookData); err != nil {
		logger.WarnX(pkg.ModuleName, "[AgentRunner] before_agent_start hook error: %v", err)
	}

	if injected, ok := hookData["injected_messages"].([]*entity.Message); ok {
		return injected
	}
	return nil
}

// fireAgentEnd fires the agent_end hook.
func (r *AgentRunner) fireAgentEnd(ctx context.Context, agent *entity.Agent, session *entity.Session, run *entity.Run) {
	hookData := map[string]interface{}{
		"agent":   agent,
		"session": session,
		"run":     run,
	}
	if err := plugin.FireHooks(ctx, r.pluginFramework.Registry(), plugin.HookAgentEnd, hookData); err != nil {
		logger.WarnX(pkg.ModuleName, "[AgentRunner] agent_end hook error: %v", err)
	}
}

// registerRun adds an abort controller to the active runs map.
func (r *AgentRunner) registerRun(runID string, ac *AbortController) {
	r.mu.Lock()
	r.activeRuns[runID] = ac
	r.mu.Unlock()
}

// unregisterRun removes an abort controller from the active runs map.
func (r *AgentRunner) unregisterRun(runID string) {
	r.mu.Lock()
	delete(r.activeRuns, runID)
	r.mu.Unlock()
}

// Abort cancels a running agent execution by run ID.
//
// If the run is active, the abort controller is triggered and the run
// transitions to Cancelled state. If the run is not found in the active
// map, it may have already completed or was never started.
//
// This pattern is aligned with subAgentManagerImpl.Cancel() which uses
// mu + map[string]CancelFunc for sub-agent abort tracking.
func (r *AgentRunner) Abort(ctx context.Context, runID string) error {
	r.mu.Lock()
	ac, ok := r.activeRuns[runID]
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("run %q not found in active runs (may have already completed)", runID)
	}

	// Trigger abort — idempotent, safe to call multiple times.
	ac.Abort()

	// Transition the run to Cancelled state if possible.
	run, err := r.runRepo.Get(ctx, runID)
	if err != nil {
		logger.Warn("[AgentRunner] Abort: failed to load run %s for state transition: %v", runID, err)
		return nil // Abort signal sent regardless.
	}

	if run.Status == entity.RunStatusInProgress {
		sm := NewRunStateMachine(run, r.runRepo)
		sm.TransitionToCancelled()
		if updateErr := r.runRepo.Update(ctx, run); updateErr != nil {
			logger.Warn("[AgentRunner] Abort: failed to persist cancelled state for run %s: %v", runID, updateErr)
		}
	}

	logger.Info("[AgentRunner] run %s aborted by external caller", runID)
	return nil
}

// buildPromptContext creates a PromptContext from the current run state.
// This bridges entity types and the prompt package's cycle-free types,
// and enriches the context with tool summaries for the ToolingSection.
func (r *AgentRunner) buildPromptContext(
	agent *entity.Agent,
	session *entity.Session,
	tools []tool.BaseTool,
) *prompt.PromptContext {
	pc := &prompt.PromptContext{
		Mode: prompt.PromptMode(agent.EffectivePromptMode()),
		Now:  time.Now(),
	}

	// Map Agent → AgentPromptInfo.
	if agent != nil {
		info := &prompt.AgentPromptInfo{
			ID:           agent.ID,
			Name:         agent.Name,
			SystemPrompt: agent.SystemPrompt,
		}
		if agent.Persona != nil {
			info.Persona = &prompt.AgentPersonaInfo{
				PromptMode:   agent.Persona.PromptMode,
				WorkspaceDir: agent.Persona.WorkspaceDir,
			}
			if agent.Persona.Identity != nil {
				info.Persona.Identity = &prompt.AgentIdentityInfo{
					Name:     agent.Persona.Identity.Name,
					Emoji:    agent.Persona.Identity.Emoji,
					Creature: agent.Persona.Identity.Creature,
					Vibe:     agent.Persona.Identity.Vibe,
					Theme:    agent.Persona.Identity.Theme,
				}
			}
		}
		pc.Agent = info
		pc.ModelName = agent.ModelRef.String()
	}

	// Session.
	if session != nil {
		pc.SessionID = session.ID
	}

	// Build tool summaries from Eino tools for the ToolingSection.
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil || info == nil {
			continue
		}
		source := "plugin"
		if len(info.Name) > 0 {
			pc.Tools = append(pc.Tools, prompt.ToolSummary{
				Name:        info.Name,
				Description: info.Desc,
				Source:      source,
			})
		}
	}

	return pc
}

// RunSubAgent executes a sub-agent run with minimal prompt mode.
//
// This is a variant of Run specifically for sub-agent execution:
//   - Forces PromptMode to "minimal" (only essential sections)
//   - The input is the sub-agent system prompt+task (already formatted by SubAgentManager)
//   - The agent's original SystemPrompt is Not used (replaced by sub-agent instructions)
//
// Modeled after OpenClaw's sub-agent execution with promptMode='minimal'.
func (r *AgentRunner) RunSubAgent(ctx context.Context, req *RunRequest) (*schema.StreamReader[*entity.AgentEvent], error) {
	// Mark request as subagent run (using a context value)
	ctx = context.WithValue(ctx, ctxKeySubAgent, true)
	return r.Run(ctx, req)
}

// ctxKey is a private type for context keys to avoid collisions.
type ctxKey string

const (
	ctxKeySubAgent        ctxKey = "subagent"
	ctxKeyParentSessionID ctxKey = "parent_session_id"
	ctxKeyParentRunID     ctxKey = "parent_run_id"
)

// isSubAgentRun checks if this run is a sub-agent execution.
func isSubAgentRun(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeySubAgent).(bool)
	return v
}

// WithParentContext returns a context with parent session and run ids,
// Called during executeRun to propagate session/run info to plugin tool handlers
func WithParentContext(ctx context.Context, sessionID, runID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyParentSessionID, sessionID)
	ctx = context.WithValue(ctx, ctxKeyParentRunID, runID)
	return ctx
}

// ParentSessionIDFromContext extracts the parent sessionID from context
func ParentSessionIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyParentSessionID).(string)
	return v
}

// ParentRunIDFromContext extracts the parent runID from context.
func ParentRunIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyParentRunID).(string)
	return v
}
