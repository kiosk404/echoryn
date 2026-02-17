package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/repo"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/agentflow"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
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
// This is the Eidolon equivalent of:
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
	defaultMaxTurns int
	runTimeout      time.Duration
}

// AgentRunnerConfig holds configuration for the AgentRunner.
type AgentRunnerConfig struct {
	DefaultMaxTurns     int
	RunTimeout          time.Duration
	MaxRetries          int
	MaxHistoryTurns     int
	CompactionThreshold float64
	KeepRecentTurns     int
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
	if cfg.DefaultMaxTurns <= 0 {
		cfg.DefaultMaxTurns = 10
	}
	if cfg.RunTimeout <= 0 {
		cfg.RunTimeout = 5 * time.Minute
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
	compactor := NewCompactor(estimator, compactorCfg)

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
		defaultMaxTurns: cfg.DefaultMaxTurns,
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

	// Build LLM context with pruning.
	buildResult := r.contextBuilder.Build(agent, session, userInput, injectedMessages, windowInfo, promptCtx)
	messages := buildResult.Messages

	logger.DebugX(pkg.ModuleName, "[AgentRunner] context built: %d messages, ~%d tokens, window=%d usable=%d",
		len(messages), buildResult.EstimatedTokens, windowInfo.WindowSize, windowInfo.UsableTokens)

	maxTurns := agent.EffectiveMaxTurns(r.defaultMaxTurns)

	// Execute the turn.
	result, err := r.turnExecutor.Execute(ctx, &TurnRequest{
		Agent:       agent,
		Messages:    messages,
		Tools:       tools,
		MaxTurns:    maxTurns,
		EventWriter: sw,
		Session:     session,
		WindowInfo:  windowInfo,
		Compactor:   r.compactor,
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

	_, err = r.compactor.Compact(ctx, session, compactModel, windowInfo)
	if err != nil {
		logger.WarnX(pkg.ModuleName, "[AgentRunner] proactive compaction failed: %v", err)
		return
	}

	_ = r.sessionRepo.Update(ctx, session)

	logger.InfoX(pkg.ModuleName, "[AgentRunner] proactive compaction completed for session %s (count=%d)",
		session.ID, session.CompactionCount)
}

// resolveSession loads an existing session or creates a new one.
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

	session := &entity.Session{
		ID:        uuid.New().String(),
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

// Abort cancels a running agent execution by run ID.
// Note: In the current implementation, abort controllers are not tracked externally.
// This is a placeholder for future implementation where run→abort mappings are maintained.
func (r *AgentRunner) Abort(_ context.Context, _ string) error {
	return fmt.Errorf("abort not yet implemented for external callers")
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
