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
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/subagent"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/toolloop"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm"
	"github.com/kiosk404/echoryn/internal/hivemind/service/mcp"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
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

	// IsTrigger indicates this run was triggered internally (e.g., by a sub-agent
	// announcement via direct delivery). When true, the Input is NOT saved as a
	// user message in session history, preventing conversation pollution.
	//
	// BUG FIX: Previous implementation always saved the triggerMessage as a user
	// message via session.AppendMessage(entity.NewUserMessage(userInput)), which
	// polluted the conversation with artificial "[subagent-announce-trigger]" messages.
	IsTrigger bool
}

// TriggerDeliverer is an optional interface for delivering trigger responses
// back to IM channels (e.g. Feishu). When a sub-agent completes and triggers
// a new turn on the parent session. this interface is used to deliver the
// response to the original IM channel.
//
// This breaks the circular dependency between AgentRunner and the gateway module.
type TriggerDeliverer interface {
	// DeliverTrigger consumes the AgentEvent stream and delivers the response
	// to the IM channel identified by sessionID. The sessionID format is
	// "{channel_id}:{chat_id}" for IM-triggered sessions.
	DeliverTrigger(ctx context.Context, sessionID string, sr *schema.StreamReader[*entity.AgentEvent])
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
	toolPolicy      *plugin.ToolPolicyPipeline
	runTimeout      time.Duration

	// subAgentMgr provides access to the Announcer for
	// Active Run tracking (MarkRunActive/Idle) and pending queue draining.
	// Set via SetSubAgentManager after construction (circular dependency break).
	subAgentMgr subagent.Manager

	// triggerDeliverer delivers trigger responses to IM channels.
	// Set via SetTriggerDeliverer after construction.
	// Optional: if nil, trigger responses are only persisted to session history.
	TriggerDeliverer TriggerDeliverer

	// activeRuns tracks in-flight abort controllers by run ID.
	// This enables external callers to cancel a running execution via Abort().
	// Pattern borrowed from subAgentManagerImpl.abortFuncs.
	mu         sync.Mutex
	activeRuns map[string]*AbortController

	// lastRunIDs maps sessionID → most recent run ID (for HTTP header exposure).
	lastRunIDs sync.Map // string → string

	// sessionActors provides per-session serialization using the Actor model.
	//
	// Aligned with OpenClaw's session lane pattern (session:<key>, maxConcurrent=1):
	// each session gets a dedicated goroutine that processes Run requests sequentially.
	// This replaces the previous sessionRunMutex which required careful defer ordering
	// to avoid deadlocks between sessionRelease and MarkRunIdle.
	//
	// With the actor model:
	//   - No explicit Lock/Unlock or defer ordering needed (deadlock-free)
	//   - Tasks are FIFO-queued and executed one at a time per session
	//   - Different sessions run fully concurrently
	//   - triggerAgentTurn just submits to the actor queue (no Acquire blocking)
	sessionActors *sessionActorPool
}

// LastRunID returns the most recently created run ID for the given session.
// This is used by the HTTP handler layer to set X-Run-ID response headers.
func (r *AgentRunner) LastRunID(sessionID string) string {
	if id, ok := r.lastRunIDs.Load(sessionID); ok {
		return id.(string)
	}
	return ""
}

// AgentRunnerConfig holds configuration for the AgentRunner.
type AgentRunnerConfig struct {
	RunTimeout          time.Duration
	MaxRetries          int
	MaxHistoryTurns     int
	CompactionThreshold float64
	KeepRecentTurns     int
	LoopDetection       toolloop.Config
	// WorkspaceDir is the resolved workspace directory (e.g. ~/.echoryn/workspace).
	// Convention prompt files (SOUL.md, IDENTITY.md, AGENTS.md, prompts/*.md)
	// are read directly from this directory.
	WorkspaceDir string

	// ToolsOptions holds tool-level policy configuration (profile, allow/deny, per-provider rules).
	// Applied to every agent turn via the ToolPolicyPipeline.
	ToolsOptions genericoptions.ToolsOptions
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

	// Build the 6-layer ToolPolicyPipeline from ToolsOptions config.
	// This replaces the hardcoded subagent.FilterDeniedTools with a unified,
	// configurable policy pipeline (Profile → Provider → Global → Agent → Channel → SubAgent).
	toolPolicy := buildToolPolicyPipeline(cfg.ToolsOptions)

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
		toolPolicy:      toolPolicy,
		runTimeout:      cfg.RunTimeout,
		activeRuns:      make(map[string]*AbortController),
		sessionActors:   newSessionActorPool(),
	}
}

// SetSubAgentManager sets the SubAgentManager reference.
// This breaks the circular dependency between AgentRunner ↔ SubAgentManager:
// the runner is created first, then the SubAgentManager is created with the runner,
// and finally the manager is set back on the runner.
//
// K8S analogy: post-start injection, similar to how controllers are wired after
// the informer factory is created.
//
// After setting the manager, this method:
//  1. Wires the AgentExecutor on the SessionController, enabling trigger functionality
//  2. Starts the SessionController's trigger worker goroutine
//
// Panics if mgr is nil or if mgr.Controller() returns nil, since a partially
// initialised SubAgentManager would cause nil-pointer panics deep in executeRun.
// Fail-fast at wiring time makes the root cause obvious.
func (r *AgentRunner) SetSubAgentManager(mgr subagent.Manager) {
	if mgr == nil {
		panic("AgentRunner.SetSubAgentManager: mgr must not be nil")
	}
	if mgr.Controller() == nil {
		panic("AgentRunner.SetSubAgentManager: mgr.Controller() must not be nil")
	}
	r.subAgentMgr = mgr

	// Wire the AgentExecutor adapter: bridges the new subagent.AgentExecutor
	// interface to the AgentRunner's Run/RunSubAgent methods.
	mgr.SetExecutor(&agentExecutorAdapter{runner: r})

	// Start the trigger worker — processes the K8s-style workqueue that
	// dedup-triggers new agent turns when sub-agents complete and the
	// parent is idle.
	mgr.Controller().StartTriggerWorker(context.Background())
}

// SetTriggerDeliverer sets the deliverer for trigger responses.
// This is called by the gateway module after the AgentRunner is constructed.
// If nil, trigger responses are only persisted to session history (not sent to IM).
func (r *AgentRunner) SetTriggerDeliverer(d TriggerDeliverer) {
	r.TriggerDeliverer = d
}

// triggerAgentTurn is called by the SessionController's trigger worker when
// a sub-agent completes and the parent session is idle. It submits a new
// agent turn on the parent session so the parent agent can process the
// sub-agent's result and respond to the user.
//
// Aligned with OpenClaw's callGateway({ method: "agent" }) in subagent-announce.ts.
//
// With the Actor model, r.Run() submits to the session's actor queue and returns
// immediately — no Acquire() blocking, no deadlock risk. The actual execution
// happens asynchronously in the session actor goroutine.
func (r *AgentRunner) triggerAgentTurn(ctx context.Context, parentSessionID, triggerMessage string) {
	// Resolve the parent session to get the agent ID.
	session, err := r.sessionRepo.Get(ctx, parentSessionID)
	if err != nil {
		logger.Warn("[AgentRunner] triggerAgentTurn: failed to get session %s: %v", parentSessionID, err)
		return
	}

	logger.Info("[AgentRunner] triggerAgentTurn: starting new turn for session %s (agent=%s)",
		parentSessionID, session.AgentID)

	// Fire a new Run on the parent session.
	// IsTrigger=true prevents the triggerMessage from being saved as a user message.
	sr, err := r.Run(ctx, &RunRequest{
		AgentID:   session.AgentID,
		SessionID: parentSessionID,
		Input:     triggerMessage,
		IsTrigger: true,
	})
	if err != nil {
		logger.Warn("[AgentRunner] triggerAgentTurn: failed to run on session %s: %v", parentSessionID, err)
		return
	}

	// If a TriggerDeliverer is set, deliver the response to the IM Channel.
	// This enables Feishu and other IM integrations to receive trigger response.
	if r.TriggerDeliverer != nil {
		safego.Go(ctx, func() {
			r.TriggerDeliverer.DeliverTrigger(ctx, parentSessionID, sr)
			logger.Info("[AgentRunner] triggerAgentTurn: delivered response for session %s", parentSessionID)
		})
		return
	}

	// Fallback: consume the stream (fire-and-forget)
	// The run's output is persisted to session history by executeRun.
	safego.Go(ctx, func() {
		for {
			_, recvErr := sr.Recv()
			if recvErr != nil {
				break
			}
		}
		logger.Info("[AgentRunner] triggerAgentTurn: completed for session %s", parentSessionID)
	})
}

// Run executes an agent interaction and returns a streaming event reader.
//
// Callers consume events via sr.Recv() until io.EOF.
//
// Session serialization uses the Actor model (aligned with OpenClaw's session lane):
// each session has a dedicated goroutine (actor) that processes runs sequentially.
// The Run request is submitted to the actor's mailbox; the actor executes it after
// all previously submitted runs for this session complete. This eliminates explicit
// Lock/Unlock and the defer-ordering deadlocks of the previous sessionRunMutex approach.
//
// Architecture:
//
//	Run(req) → sessionActors.Submit(sessionID, task)
//	  → task is queued in the session's mailbox channel
//	  → session actor goroutine dequeues and executes sequentially
//	  → BoltDB deep-copy isolation is preserved (each run sees latest state)
//	  → no Lock/Unlock, no defer ordering concerns
func (r *AgentRunner) Run(ctx context.Context, req *RunRequest) (*schema.StreamReader[*entity.AgentEvent], error) {
	// 1. Resolve agent (outside actor — read-only, no session contention).
	agent, err := r.agentRepo.Get(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agent %q: %w", req.AgentID, err)
	}

	// 2. Assign session ID if not provided.
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
		req.SessionID = sessionID
	}

	// 3. Create streaming event pipe.
	// Buffer=200 to prevent deadlock between fast producer and slow consumer.
	sr, sw := schema.Pipe[*entity.AgentEvent](200)

	// 4. Submit the run to the session's actor for serial execution.
	// This is the Go equivalent of OpenClaw's:
	//   enqueueCommandInLane("session:<key>", task)
	//
	// The task closure captures all state needed for execution. The actor guarantees
	// that only one task runs per session at a time, eliminating the need for
	// sessionRunLock and its associated defer-ordering complexity.
	r.sessionActors.Submit(sessionID, func() {
		r.runInActor(ctx, agent, req, sw)
	})

	return sr, nil
}

// runInActor is the task body executed inside the session actor goroutine.
// It is always called sequentially for a given session — the actor guarantees
// mutual exclusion without explicit locks.
//
// All session-mutating operations (resolve, append messages, update) happen here,
// ensuring single-writer semantics via the actor's serialization.
func (r *AgentRunner) runInActor(
	ctx context.Context,
	agent *entity.Agent,
	req *RunRequest,
	sw *schema.StreamWriter[*entity.AgentEvent],
) {
	// Resolve or create session (safe — we're the only goroutine for this session).
	session, err := r.resolveSession(ctx, agent, req.SessionID)
	if err != nil {
		sw.Send(&entity.AgentEvent{
			Type:  entity.EventError,
			Error: fmt.Sprintf("session resolution failed: %v", err),
		}, nil)
		sw.Close()
		return
	}

	// Create run record.
	run := &entity.Run{
		ID:        uuid.New().String(),
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    entity.RunStatusCreated,
		Input:     req.Input,
		CreatedAt: time.Now(),
	}
	if err := r.runRepo.Create(ctx, run); err != nil {
		sw.Send(&entity.AgentEvent{
			Type:  entity.EventError,
			Error: fmt.Sprintf("failed to create run: %v", err),
		}, nil)
		sw.Close()
		return
	}

	// Create state machine.
	stateMachine := NewRunStateMachine(run)
	if err := stateMachine.TransitionToInProgress(); err != nil {
		sw.Send(&entity.AgentEvent{
			Type:  entity.EventError,
			Error: err.Error(),
		}, nil)
		sw.Close()
		return
	}

	// Create abort controller.
	abort := NewAbortController(ctx, run.ID, r.runTimeout)

	// Register run for external abort support.
	r.registerRun(run.ID, abort)

	// Store session -> run ID mapping for X-Run-ID response header exposure.
	r.lastRunIDs.Swap(req.SessionID, run.ID)

	// Emit initial run status event.
	sw.Send(&entity.AgentEvent{
		Type:      entity.EventRunStatus,
		RunStatus: entity.RunStatusInProgress,
	}, nil)

	// Execute the run synchronously within the actor goroutine.
	// This is the key difference from the old approach: the entire executeRun
	// happens inside the actor, so no separate goroutine or lock release is needed.
	r.executeRun(abort.Context(), agent, session, run, stateMachine, sw, abort, req.Input, req.IsTrigger)

	// Post-run cleanup: MarkRunIdle + retrigger.
	// No defer-ordering concerns — we execute these sequentially after executeRun.
	// The actor guarantees no other run for this session can start until we finish.
	if r.subAgentMgr != nil {
		pending := r.subAgentMgr.Controller().MarkRunIdle(session.ID)
		if len(pending) > 0 {
			logger.Info("[AgentRunner] %d late pending announcements for session %s, re-enqueuing trigger",
				len(pending), session.ID)
			r.subAgentMgr.Controller().ReEnqueuePending(session.ID, pending)
		}
	}

	// Clean up resources.
	r.unregisterRun(run.ID)
	sw.Close()
	abort.CleanUp()
}

// executeRun is the core execution body, called synchronously inside the session actor.
func (r *AgentRunner) executeRun(
	ctx context.Context,
	agent *entity.Agent,
	session *entity.Session,
	run *entity.Run,
	stateMachine *RunStateMachine,
	sw *schema.StreamWriter[*entity.AgentEvent],
	abort *AbortController,
	userInput string,
	isTrigger bool,
) {
	// Propagate session/run IDs into context so that downstream plugin tool
	// handlers (e.g. subagent's sessions_spawn) can retrieve the parent
	// session/run via ParentSessionIDFromContext / ParentRunIDFromContext.
	ctx = WithParentContext(ctx, session.ID, run.ID)

	// Mark this session as having an active run and get the steer channel.
	// This enables the SessionController to use the correct delivery path
	// (steer/pending) for sub-agent announcements that arrive during this run.
	// The steer channel is passed to TurnExecutor → AgentFlowBuilder → SteerAwareChatModel
	// which drains it before each LLM call for real-time sub-agent result injection.
	//
	// NOTE: MarkRunIdle is called in runInActor() after executeRun returns.
	// The actor model guarantees sequential execution, so no defer-ordering
	// concerns exist — MarkRunIdle runs after all session writes are complete
	// and before the next run for this session can start.
	var steerCh <-chan string
	if r.subAgentMgr != nil {
		sc := r.subAgentMgr.Controller().MarkRunActive(session.ID)
		if sc != nil {
			steerCh = sc.Ch
		}
	}

	// Fire before_agent_start hook (memory injection, etc.).
	injectedMessages := r.fireBeforeAgentStart(ctx, agent, session)

	// Resolve context window.
	windowInfo := r.resolveWindowInfo(ctx, agent)

	// Apply 6-layer ToolPolicyPipeline on raw ToolDefinitions BEFORE adaptation.
	// This replaces the old hardcoded subagent.FilterDeniedTools with a unified,
	// configurable pipeline (Profile → Provider → Global → Agent → Channel → SubAgent).
	policyCtx := plugin.PolicyContext{
		AgentID:    agent.ID,
		IsSubAgent: isSubAgentRun(ctx),
	}
	filteredDefs := r.applyToolPolicy(policyCtx)

	// Adapt policy-filtered plugin tools to Eino tools.
	// AdaptResult separates active (full schema) from deferred (name-only) tools.
	adaptResult := agentflow.AdaptPluginToolsFromDefs(filteredDefs)
	pluginTools := adaptResult.ActiveTools

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
			logger.DebugX(pkg.ModuleName, "[AgentRunner] merged %d plugin tools + %d MCP tools (deferred=%d)",
				len(pluginTools), len(mcpToolsList), len(adaptResult.DeferredNames))
		}
	}

	// Build PromptContext with tool summaries for the PromptPipeline.
	// Inject deferred tool names so ToolingSection can list them in the system prompt.
	promptCtx := r.buildPromptContext(agent, session, tools, adaptResult.DeferredNames)
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
		SteerCh:      steerCh,
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

	// Persist: update session history with full conversation context.
	//
	// The session must contain the COMPLETE conversation for this turn:
	//   1. User input message (SKIPPED when IsTrigger=true to prevent pollution)
	//   2. Intermediate messages (assistant tool_calls + tool results from ReAct loop)
	//   3. Steer messages (sub-agent results injected via Steer delivery path)
	//   4. Pending announcements (sub-agent results that arrived while run was active)
	//   5. Final assistant text response
	//
	// v2 CRITICAL: Step 4 is the key architectural change. Previously, pending
	// announcements were written to the session by the Announcer's deliverDirect()
	// using a separate lock (SessionWriteLock), causing lost updates when the runner
	// also writes using sessionRunLock. Now, the runner is the SOLE writer:
	// pending announcements are consumed from the SessionController and appended
	// to the runner's own session object, then persisted in a single Update() call.
	//
	// BUG FIX: When IsTrigger=true (e.g., sub-agent announce triggered this run),
	// the userInput is a system trigger like "[subagent-announce-trigger]" and must
	// NOT be saved as a user message. Previous implementation always saved it,
	// causing artificial messages to appear in conversation history.
	if !isTrigger {
		session.AppendMessage(entity.NewUserMessage(userInput))
	}
	if len(result.IntermediateMessages) > 0 {
		session.AppendMessages(FromSchemaMessages(result.IntermediateMessages))
		logger.InfoX(pkg.ModuleName, "[AgentRunner] persisted %d intermediate messages (tool_calls + tool_results) to session %s",
			len(result.IntermediateMessages), session.ID)
	}
	if len(result.SteerMessages) > 0 {
		session.AppendMessages(FromSchemaMessages(result.SteerMessages))
		logger.InfoX(pkg.ModuleName, "[AgentRunner] persisted %d steer messages (sub-agent results) to session %s",
			len(result.SteerMessages), session.ID)
	}

	// v2: Consume pending announcements from SessionController and append to session.
	// This is the SINGLE-WRITER pattern: the runner reads pending from the controller
	// (which only stores them in memory) and writes them to the session object that
	// the runner exclusively owns. No other goroutine writes to this session object.
	if r.subAgentMgr != nil {
		pending := r.subAgentMgr.Controller().TakePending(session.ID)
		if len(pending) > 0 {
			pendingMsgs := subagent.FormatAnnouncementsAsMessages(pending)
			session.AppendMessages(pendingMsgs)
			logger.InfoX(pkg.ModuleName, "[AgentRunner] consumed %d pending announcements for session %s",
				len(pending), session.ID)
		}
	}

	session.AppendMessage(entity.NewAssistantMessage(finalContent))
	session.AddUsage(result.Usage)
	if err := r.sessionRepo.Update(ctx, session); err != nil {
		logger.Warn("[AgentRunner] failed to persist session %s: %v", session.ID, err)
	}

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

// fireAfterTurnHooks fires all registered after-turn hooks.
// These run after a successful turn, before the Done event is emitted.
// Hook failures are logged but do not fail the run.
//
// Aligned with OpenClaw's ContextEngine.afterTurn() and Claude Code's PostSamplingHook.
func (r *AgentRunner) fireAfterTurnHooks(
	ctx context.Context,
	agent *entity.Agent,
	session *entity.Session,
	run *entity.Run,
	usage *entity.TokenUsage,
) {
	hooks := r.pluginFramework.Registry().GetAfterTurnHooks()
	if len(hooks) == 0 {
		return
	}

	data := plugin.AfterTurnData{
		AgentID:   agent.ID,
		SessionID: session.ID,
		RunID:     run.ID,
		Messages:  session.ActiveMessages(),
	}
	if usage != nil {
		data.TokensUsed = int(usage.TotalTokens)
	}

	for _, hook := range hooks {
		if err := hook(ctx, data); err != nil {
			logger.WarnX(pkg.ModuleName, "[AgentRunner] after-turn hook error: %v", err)
		}
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
		sm := NewRunStateMachine(run)
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
	deferredNames []string,
) *prompt.PromptContext {
	// Build base PromptContext from agent/session using the shared helper.
	pc := BuildPromptContextFromAgent(agent, session)

	// Set ModelName from agent's ModelRef.
	if agent != nil {
		pc.ModelName = agent.ModelRef.String()
	}

	// Enrich with tool summaries from Eino tools for the ToolingSection.
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

	// Inject deferred tool names so ToolingSection can list them as "available via tool_search".
	if len(deferredNames) > 0 {
		if pc.Extra == nil {
			pc.Extra = make(map[string]interface{})
		}
		pc.Extra["deferred_tools"] = deferredNames
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

// agentExecutorAdapter adapts AgentRunner to the subagent.AgentExecutor interface.
//
// This bridge enables the subagent package to call back into the AgentRunner
// without importing it (which would create a circular dependency). The adapter
// translates between the subagent package's ExecuteRequest and the runtime
// package's RunRequest/methods.
type agentExecutorAdapter struct {
	runner *AgentRunner
}

var _ subagent.AgentExecutor = (*agentExecutorAdapter)(nil)

// RunSubAgent starts a sub-agent run via the AgentRunner.
func (a *agentExecutorAdapter) RunSubAgent(ctx context.Context, req *subagent.ExecuteRequest) (*schema.StreamReader[*entity.AgentEvent], error) {
	return a.runner.RunSubAgent(ctx, &RunRequest{
		AgentID:   req.AgentID,
		SessionID: req.SessionID,
		Input:     req.Input,
	})
}

// TriggerParentTurn triggers a new agent turn on the parent session.
// Uses the AgentRunner's triggerAgentTurn which has IsTrigger semantics.
func (a *agentExecutorAdapter) TriggerParentTurn(ctx context.Context, parentSessionID, triggerMessage string) {
	a.runner.triggerAgentTurn(ctx, parentSessionID, triggerMessage)
}

// buildToolPolicyPipeline constructs a 6-layer ToolPolicyPipeline from ToolsOptions.
// This converts the config-level ToolsOptions (with JSON-friendly ToolAllowDeny)
// to the plugin-level AllowDeny types used by the pipeline layers.
func buildToolPolicyPipeline(opts genericoptions.ToolsOptions) *plugin.ToolPolicyPipeline {
	// Convert by_provider map from config types to plugin types.
	byProvider := make(map[string]plugin.AllowDeny, len(opts.ByProvider))
	for providerID, ad := range opts.ByProvider {
		byProvider[providerID] = plugin.AllowDeny{
			Allow: ad.Allow,
			Deny:  ad.Deny,
		}
	}

	globalAD := plugin.AllowDeny{
		Allow: opts.Allow,
		Deny:  opts.Deny,
	}

	return plugin.NewDefaultPolicyPipeline(globalAD, byProvider)
}

// applyToolPolicy runs the ToolPolicyPipeline on all registered plugin tools,
// returning the policy-filtered ToolDefinition list. The pipeline applies
// Profile, Provider, Global, Agent, Channel, and SubAgent layers in order.
func (r *AgentRunner) applyToolPolicy(policyCtx plugin.PolicyContext) []plugin.ToolDefinition {
	allTools := r.pluginFramework.Registry().GetTools()

	// Collect all tool definitions into a slice for the pipeline.
	defs := make([]plugin.ToolDefinition, 0, len(allTools))
	for _, def := range allTools {
		defs = append(defs, def)
	}

	if r.toolPolicy == nil {
		return defs
	}

	filtered := r.toolPolicy.Apply(defs, policyCtx)
	if dropped := len(defs) - len(filtered); dropped > 0 {
		logger.DebugX(pkg.ModuleName, "[AgentRunner] ToolPolicyPipeline: %d/%d tools passed (dropped %d)",
			len(filtered), len(defs), dropped)
	}
	return filtered
}
