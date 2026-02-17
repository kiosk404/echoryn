package runtime

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// ContextBuilder assembles the LLM input message list from various sources,
// applying history limits, compaction summaries, and context pruning.
//
// The final message list follows this order:
//  1. System prompt (from PromptPipeline if available, else Agent.SystemPrompt)
//  2. Compaction summary (if session was compacted)
//  3. Memory-injected messages (from plugin hooks, if any)
//  4. Session history (active messages only, limited by MaxHistoryTurns)
//  5. Current user input
//
// After assembly, the message list is pruned to fit within the context window.
//
// This is the Eidolon equivalent of OpenClaw's context building pipeline
// combined with airi-go's prompt template assembly.
type ContextBuilder struct {
	estimator       *TokenEstimator
	pruner          *ContextPruner
	maxHistoryTurns int
	pipeline        *prompt.Pipeline
}

// NewContextBuilder creates a new ContextBuilder.
// maxHistoryTurns limits how many recent user turns of history to include (0 = no limit).
// pipeline may be nil for backward compatibility (uses agent.SystemPrompt directly).
func NewContextBuilder(estimator *TokenEstimator, pruner *ContextPruner, maxHistoryTurns int) *ContextBuilder {
	return &ContextBuilder{
		estimator:       estimator,
		pruner:          pruner,
		maxHistoryTurns: maxHistoryTurns,
	}
}

// SetPipeline attaches a PromptPipeline to the ContextBuilder.
// When set, Build() uses the pipeline to assemble the system prompt
// instead of using agent.SystemPrompt directly.
func (cb *ContextBuilder) SetPipeline(p *prompt.Pipeline) {
	cb.pipeline = p
}

// Pipeline returns the attached PromptPipeline (may be nil).
func (cb *ContextBuilder) Pipeline() *prompt.Pipeline {
	return cb.pipeline
}

// BuildResult holds the assembled and pruned context.
type BuildResult struct {
	Messages         []*schema.Message
	EstimatedTokens  int
	HistoryTrimmed   bool
	PruneSoftTrimmed int
	PruneHardCleared int
}

// Build assembles the complete message list for LLM consumption.
//
// When a PromptPipeline is attached (via SetPipeline), the system prompt is
// assembled dynamically from registered Sections. Otherwise, falls back to
// using agent.SystemPrompt directly (backward compatibility).
//
// The optional promptCtx parameter allows callers to pass a pre-built PromptContext.
// If nil, a minimal PromptContext is constructed from the agent and session.
func (cb *ContextBuilder) Build(
	agent *entity.Agent,
	session *entity.Session,
	userInput string,
	injectedMessages []*entity.Message,
	windowInfo ContextWindowInfo,
	promptCtx ...*prompt.PromptContext,
) BuildResult {
	var messages []*schema.Message

	// 1. System prompt — use Pipeline if available, else raw agent.SystemPrompt.
	systemPrompt := cb.resolveSystemPrompt(agent, session, promptCtx...)
	if systemPrompt != "" {
		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: systemPrompt,
		})
	}

	// 2. Compaction summary (if session was compacted previously).
	if session != nil && session.HasCompaction() {
		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: fmt.Sprintf("[Conversation Summary]\n%s", session.CompactionSummary),
		})
	}

	// 3. Memory-injected messages (e.g., from memory plugin's before_agent_start hook).
	if len(injectedMessages) > 0 {
		messages = append(messages, ToSchemaMessages(injectedMessages)...)
	}

	// 4. Session history (only active messages, with turn limit).
	historyTrimmed := false
	if session != nil {
		activeMessages := session.ActiveMessages()
		if len(activeMessages) > 0 {
			historyMsgs := activeMessages
			if cb.maxHistoryTurns > 0 {
				historyMsgs, historyTrimmed = cb.limitHistoryTurns(activeMessages)
			}
			messages = append(messages, ToSchemaMessages(historyMsgs)...)
		}
	}

	// 5. Current user input.
	if userInput != "" {
		messages = append(messages, &schema.Message{
			Role:    schema.User,
			Content: userInput,
		})
	}

	// 6. Apply context pruning.
	pruneResult := cb.pruner.Prune(messages, windowInfo.UsableTokens)

	if pruneResult.SoftTrimmed > 0 || pruneResult.HardCleared > 0 {
		logger.Info("[ContextBuilder] pruning applied: soft_trimmed=%d, hard_cleared=%d, tokens=%d/%d",
			pruneResult.SoftTrimmed, pruneResult.HardCleared,
			pruneResult.EstimatedTokens, windowInfo.UsableTokens)
	}

	return BuildResult{
		Messages:         pruneResult.Messages,
		EstimatedTokens:  pruneResult.EstimatedTokens,
		HistoryTrimmed:   historyTrimmed,
		PruneSoftTrimmed: pruneResult.SoftTrimmed,
		PruneHardCleared: pruneResult.HardCleared,
	}
}

// limitHistoryTurns returns the last N user turns (and all associated messages
// between them, including assistant replies and tool calls/results).
//
// This is the Eidolon equivalent of OpenClaw's limitHistoryTurns().
func (cb *ContextBuilder) limitHistoryTurns(messages []*entity.Message) ([]*entity.Message, bool) {
	if cb.maxHistoryTurns <= 0 {
		return messages, false
	}

	// Count user turns from the end to find the cutoff.
	userTurns := 0
	cutoff := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == entity.RoleUser {
			userTurns++
			if userTurns >= cb.maxHistoryTurns {
				cutoff = i
				break
			}
		}
	}

	if cutoff == 0 {
		return messages, false
	}

	logger.Debug("[ContextBuilder] history trimmed: keeping last %d turns (%d/%d messages)",
		cb.maxHistoryTurns, len(messages)-cutoff, len(messages))
	return messages[cutoff:], true
}

// resolveSystemPrompt assembles the system prompt text.
//
// When a PromptPipeline is attached, it uses the pipeline to render all sections.
// Otherwise, falls back to agent.SystemPrompt (backward compatibility).
func (cb *ContextBuilder) resolveSystemPrompt(
	agent *entity.Agent,
	session *entity.Session,
	promptCtx ...*prompt.PromptContext,
) string {
	if cb.pipeline == nil {
		return agent.SystemPrompt
	}

	// Use provided PromptContext or build one from the agent/session.
	var pc *prompt.PromptContext
	if len(promptCtx) > 0 && promptCtx[0] != nil {
		pc = promptCtx[0]
	} else {
		pc = cb.buildPromptContext(agent, session)
	}

	assembled, err := cb.pipeline.Assemble(context.Background(), pc)
	if err != nil {
		logger.Warn("[ContextBuilder] prompt pipeline assembly failed: %v, falling back to agent.SystemPrompt", err)
		return agent.SystemPrompt
	}

	// If pipeline produced nothing (all sections disabled/empty), fall back.
	if assembled == "" {
		return agent.SystemPrompt
	}

	return assembled
}

// buildPromptContext converts entity.Agent + entity.Session into a prompt.PromptContext.
// This bridges the entity layer and the prompt layer without import cycles.
func (cb *ContextBuilder) buildPromptContext(agent *entity.Agent, session *entity.Session) *prompt.PromptContext {
	pc := &prompt.PromptContext{
		Mode: prompt.PromptMode(agent.EffectivePromptMode()),
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
	}

	// Map Session → SessionID.
	if session != nil {
		pc.SessionID = session.ID
	}

	return pc
}
