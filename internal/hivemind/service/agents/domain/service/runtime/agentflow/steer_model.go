package agentflow

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// SteerAwareChatModel wraps a ChatModel to inject sub-agent announcements
// (steer messages) into the conversation before each LLM call.
//
// This implements the Steer delivery path aligned with OpenClaw's
// queueEmbeddedPiMessage / activeSession.steer(): when a sub-agent
// completes while the parent agent's ReAct loop is running, the result
// is pushed into the steer channel. Before each LLM call in the ReAct
// loop, this wrapper drains the channel and appends the messages as
// user-role messages, so the LLM sees the sub-agent results in its
// current turn without waiting for the next user message.
//
// IMPORTANT: Steer messages use the "user" role (not "system") to avoid
// breaking vLLM / chat-template constraints that require system messages
// to appear only at the first position. This aligns with OpenClaw where
// activeSession.steer() injects text as a user turn.
//
// Architecture:
//
//	AnnounceController.dispatch()
//	  → steerChannel.ch <- event.FormatForPrompt()
//	      ↓ (buffered channel, cap=8)
//	SteerAwareChatModel.Generate/Stream()
//	  → drain steerCh → append user messages → call inner ChatModel
//
// This is transparent to the Eino ReAct agent — it only sees a ChatModel
// that occasionally has extra user messages in its input.
//
// The wrapper implements both BaseChatModel and ToolCallingChatModel
// so it can be used with the ReAct agent which requires tool calling support.
type SteerAwareChatModel struct {
	inner   model.BaseChatModel
	steerCh <-chan string

	// parent is set on child wrappers created by WithTools.
	// When non-nil, consumed steer messages are recorded on the parent
	// so that ConsumedMessages() on the root returns ALL injections.
	parent *SteerAwareChatModel

	// consumedMu guards consumedMessages.
	consumedMu sync.Mutex
	// consumedMessages records all steer messages that were injected into LLM calls.
	// These must be persisted to session history so the agent retains context
	// of sub-agent results across turns. Without this, steer-delivered sub-agent
	// results are ephemeral and lost after the current run.
	consumedMessages []*schema.Message
}

// Compile-time interface checks.
var (
	_ model.BaseChatModel        = (*SteerAwareChatModel)(nil)
	_ model.ToolCallingChatModel = (*SteerAwareChatModel)(nil)
)

// NewSteerAwareChatModel wraps a ChatModel with steer channel awareness.
// If steerCh is nil, the wrapper is a no-op passthrough (returns inner as-is).
func NewSteerAwareChatModel(inner model.BaseChatModel, steerCh <-chan string) model.BaseChatModel {
	if steerCh == nil {
		return inner
	}
	return &SteerAwareChatModel{
		inner:   inner,
		steerCh: steerCh,
	}
}

// Generate calls the inner model's Generate, injecting any pending steer messages first.
func (m *SteerAwareChatModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	messages = m.injectSteerMessages(messages)
	return m.inner.Generate(ctx, messages, opts...)
}

// Stream calls the inner model's Stream, injecting any pending steer messages first.
func (m *SteerAwareChatModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	messages = m.injectSteerMessages(messages)

	// DEBUG: Log the messages being sent to the LLM
	logger.Debug("[SteerAwareChatModel.Stream] sending %d messages to LLM:", len(messages))
	for i, msg := range messages {
		contentPreview := msg.Content
		if len(contentPreview) > 200 {
			contentPreview = contentPreview[:200] + "..."
		}
		logger.Debug("[SteerAwareChatModel.Stream]   msg[%d] role=%s toolCalls=%d content_len=%d content=%.200s",
			i, msg.Role, len(msg.ToolCalls), len(msg.Content), contentPreview)
	}

	return m.inner.Stream(ctx, messages, opts...)
}

// WithTools delegates to the inner model's WithTools if it supports ToolCallingChatModel.
// The returned model is wrapped with the same steer channel so tool-bound models
// also benefit from steer injection.
//
// IMPORTANT: The re-wrapped model shares the SAME parent reference so that
// consumed steer messages from tool-bound calls are recorded in the original
// SteerAwareChatModel's consumedMessages. This ensures ConsumedMessages()
// returns ALL steer messages regardless of which wrapper layer injected them.
func (m *SteerAwareChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	if tcm, ok := m.inner.(model.ToolCallingChatModel); ok {
		bound, err := tcm.WithTools(tools)
		if err != nil {
			return nil, err
		}
		// Re-wrap the tool-bound model with steer awareness.
		// Share the parent's consumedMessages so all injections are tracked centrally.
		child := &SteerAwareChatModel{
			inner:   bound,
			steerCh: m.steerCh,
			parent:  m,
		}
		return child, nil
	}
	// Inner model doesn't support tool calling — return self as-is.
	// This shouldn't happen in practice because the ReAct agent requires it.
	return m, nil
}

// ConsumedMessages returns all steer messages that were injected into LLM calls
// during this run. These messages must be persisted to session history so the
// agent retains context of sub-agent results across turns.
//
// Thread-safe: can be called after the run completes.
func (m *SteerAwareChatModel) ConsumedMessages() []*schema.Message {
	m.consumedMu.Lock()
	defer m.consumedMu.Unlock()
	result := make([]*schema.Message, len(m.consumedMessages))
	copy(result, m.consumedMessages)
	return result
}

// injectSteerMessages non-blockingly drains the steer channel and appends
// any pending sub-agent announcements as a SINGLE user message to the conversation.
//
// CRITICAL: Multiple steer messages are MERGED into one user message to avoid
// breaking the user/assistant alternation pattern that LLMs expect.
//
// The user role is used (not system) for two reasons:
//  1. vLLM and many chat templates (Qwen, Mistral, etc.) require system messages
//     to appear ONLY at the first position. Inserting system messages mid-conversation
//     causes template rendering errors like "System message must be at the beginning."
//  2. This aligns with OpenClaw's design where activeSession.steer() injects
//     sub-agent results as user-role messages.
//
// The message content is prefixed with a runtime context header so the LLM can
// distinguish it from actual user input.
//
// The merged message is appended at the end of the message array so the LLM sees
// sub-agent completions as the most recent context.
//
// Returns the original messages slice if no steer messages are pending (zero allocation).
func (m *SteerAwareChatModel) injectSteerMessages(messages []*schema.Message) []*schema.Message {
	if m.steerCh == nil {
		return messages
	}

	// Non-blocking drain of all available steer messages.
	var steerMsgs []string
	for {
		select {
		case msg, ok := <-m.steerCh:
			if !ok {
				// Channel closed (run ended) — stop draining.
				goto done
			}
			steerMsgs = append(steerMsgs, msg)
		default:
			goto done
		}
	}
done:

	if len(steerMsgs) == 0 {
		return messages
	}

	logger.Info("[SteerAwareChatModel] injecting %d steer messages (merged into 1 user message) into LLM context", len(steerMsgs))

	// Merge all steer messages into a single user message.
	// This avoids injecting multiple messages which break the
	// user/assistant alternation pattern and cause garbled LLM output.
	//
	// Format: each sub-agent result is separated by a blank line for clarity.
	var merged string
	if len(steerMsgs) == 1 {
		merged = steerMsgs[0]
	} else {
		// Multiple announcements — wrap in a summary header.
		merged = "[SUBAGENT_ANNOUNCEMENTS]\n"
		for i, msg := range steerMsgs {
			if i > 0 {
				merged += "\n---\n"
			}
			merged += msg
		}
	}

	logger.Debug("[SteerAwareChatModel] merged steer message: %.200s...", merged)

	// Append as a single user message at the end.
	// User role avoids vLLM/chat-template "system must be first" constraints.
	// Aligned with OpenClaw's activeSession.steer() which injects as user turn.
	steerMsg := &schema.Message{
		Role:    schema.User,
		Content: merged,
	}

	// Record the injected steer message for later persistence to session history.
	// Without this, steer-delivered sub-agent results are ephemeral: they are
	// consumed by the LLM in the current turn but lost from session history,
	// causing the agent to forget sub-agent outputs on the next turn.
	//
	// If this is a child wrapper (created by WithTools), record on the root
	// parent so ConsumedMessages() returns all injections from one place.
	target := m
	if m.parent != nil {
		target = m.parent
	}
	target.consumedMu.Lock()
	target.consumedMessages = append(target.consumedMessages, steerMsg)
	target.consumedMu.Unlock()

	result := make([]*schema.Message, 0, len(messages)+1)
	result = append(result, messages...)
	result = append(result, steerMsg)

	return result
}
