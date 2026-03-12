package agentflow

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// MessageCollector accumulates intermediate messages (tool calls, tool results)
// generated during a ReAct agent turn. These messages are NOT part of the final
// stream result (which only contains the last assistant text), but they MUST be
// persisted to session history so the agent has full context in future turns.
//
// Without this, the session only stores:
//
//	User: "帮我写三首诗"
//	Assistant: "已启动3个子代理"
//
// But the real conversation included tool_call(sessions_spawn) + tool_result
// messages in between. On the next turn the agent has no idea what "第三个"
// refers to, causing the "继续什么？" context-loss bug.
type MessageCollector struct {
	mu       sync.Mutex
	messages []*schema.Message
}

// NewMessageCollector creates a new empty collector.
func NewMessageCollector() *MessageCollector {
	return &MessageCollector{}
}

// Append adds a message to the collector (thread-safe).
func (mc *MessageCollector) Append(msgs ...*schema.Message) {
	mc.mu.Lock()
	mc.messages = append(mc.messages, msgs...)
	mc.mu.Unlock()
}

// Messages returns all collected messages in order (thread-safe).
func (mc *MessageCollector) Messages() []*schema.Message {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	result := make([]*schema.Message, len(mc.messages))
	copy(result, mc.messages)
	return result
}

// ReplayChunkCallback is the Eino callbacks.Handler that intercepts streaming
// events from the execution graph and translates them into AgentEvent entities.
// It intercepts:
// - ChatModel stream outputs -> EventTextDelta events
// - ToolsNode start -> EventToolCall events
// - ToolsNode end -> EventToolResult events
// - Errors -> EventError events
// All events are pushed into a schema.StreamWriter[*entity.AgentEvent].
//
// Additionally, it collects intermediate tool_call and tool_result messages
// into a MessageCollector, which is used by the runner to persist full
// conversation history to the session. This prevents the context-loss bug
// where the agent forgets tool interactions between turns.
//
// An optional StreamMiddlewareChain can be attached to process each streaming
// chunk before it is translated into AgentEvent. This is aligned with OpenClaw's
// response-side stream wrappers (wrapStreamTrimToolCallNames, payloadLogger)
type ReplayChunkCallback struct {
	sw         *schema.StreamWriter[*entity.AgentEvent]
	middleware *StreamMiddlewareChain
	collector  *MessageCollector

	// wg tracks all goroutines launched by OnEndWithStreamOutput (consumeChatModelStream
	// and consumeToolsNodeStream). The executor MUST call Wait() after the graph-level
	// stream is fully consumed (collectStreamResult returns) but BEFORE reading
	// collector.Messages(). Without this, there is a race condition: the callback
	// goroutines may still be writing to the collector when Messages() is called,
	// causing incomplete IntermediateMessages (missing tool_result messages).
	// This results in malformed session history like [assistant(tool_calls),
	// assistant(tool_calls)] without intervening tool messages, which triggers
	// DeepSeek HTTP 400: "tool_calls must be followed by tool messages".
	wg sync.WaitGroup

	// consuming is a per-round dedupe flag for ChatModel stream consumption.
	//
	// Problem: Eino's callback system fires OnEndWithStreamOutput for every
	// graph layer that wraps a ChatModel (SteerAwareChatModel wrapper + inner
	// ChatModel), so each ReAct round triggers 2 callbacks for the same stream.
	// Without dedupe, every chunk is emitted twice ("HelloHello" duplication).
	//
	// The previous sync.Once approach only allowed ONE consumption across the
	// entire run, which broke ReAct multi-round loops: only the first LLM call's
	// stream was consumed; subsequent rounds (including the final text reply)
	// were all drained, causing "subagent stuck at running..." when the final
	// response was silently discarded.
	//
	// Fix: atomic.Bool acts as a CAS (compare-and-swap) per-round guard:
	//   - First ChatModel callback in a round: CAS(false→true) succeeds → consume
	//   - Second callback (same round): CAS fails → drain (it's a duplicate)
	//   - consumeChatModelStream finishes → resets to false
	//   - Next ReAct round: CAS(false→true) succeeds again → consume
	consuming atomic.Bool
}

// WithMiddleware attaches a StreamMiddlewareChain for response-side processing.
// This enables chunk-level transformations (e.g., tool call name trimming,
// payload logging) aligned with OpenClaw's stream wrapper pipeline.
func (r *ReplayChunkCallback) WithMiddleware(chain *StreamMiddlewareChain) *ReplayChunkCallback {
	r.middleware = chain
	return r
}

// NewReplayChunkCallback creates a new ReplayChunkCallback.
func NewReplayChunkCallback(sw *schema.StreamWriter[*entity.AgentEvent], collector *MessageCollector) *ReplayChunkCallback {
	return &ReplayChunkCallback{sw: sw, collector: collector}
}

// Wait blocks until all callback goroutines (consumeChatModelStream,
// consumeToolsNodeStream) have finished writing to the MessageCollector.
//
// The executor MUST call this after the graph-level stream is fully consumed
// (collectStreamResult returns) but BEFORE reading collector.Messages().
// This eliminates the race condition where goroutines are still appending
// messages to the collector while the executor reads an incomplete snapshot.
func (r *ReplayChunkCallback) Wait() {
	r.wg.Wait()
}

// Build returns the Eino callbacks.Handler that intercepts streaming events.
func (r *ReplayChunkCallback) Build() callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(r.OnStart).
		OnEndFn(r.OnEnd).
		OnEndWithStreamOutputFn(r.OnEndWithStreamOutput).
		OnErrorFn(r.OnError).
		Build()
}

// OnStart intercepts node start events.
// When a ToolsNode starts, emit a tool_call_start placeholder event.
func (r *ReplayChunkCallback) OnStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
	if info.Component == compose.ComponentOfToolsNode {
		logger.Debug("[AgentFlow/Callback] ToolsNode started: %s", info.Name)
	}
	return ctx
}

// OnEnd intercepts node completion events.
func (r *ReplayChunkCallback) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	return ctx
}

// OnEndWithStreamOutput intercepts streaming completion events.
//
// For ChatModel: reads streaming chunks and emits TextDelta + ToolCallStart events.
// For ToolsNode: collects tool results and emits ToolCallEnd events.
func (r *ReplayChunkCallback) OnEndWithStreamOutput(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	logger.Info("[AgentFlow/Callback] OnEndWithStreamOutput: component=%s name=%s", info.Component, info.Name)
	switch info.Component {
	case components.ComponentOfChatModel:
		// Eino's callback system fires OnEndWithStreamOutput at every graph layer
		// that wraps a ChatModel. For example, when SteerAwareChatModel wraps the
		// inner ChatModel, both layers report component=ChatModel, causing this
		// handler to be called twice for the same logical stream in a single
		// ReAct round. The first call gets the real stream; the second gets a
		// duplicate that produces the same chunks.
		//
		// We use atomic CAS (compare-and-swap) as a per-round guard:
		//   - First ChatModel callback in a round: CAS(false→true) → consume
		//   - Second callback (same round, duplicate): CAS fails → drain
		//   - consumeChatModelStream finishes → resets consuming to false
		//   - Next ReAct round: CAS(false→true) succeeds again
		//
		// This replaces the previous sync.Once which only allowed consumption of
		// the FIRST round's stream, causing all subsequent rounds (including the
		// final text reply after tool execution) to be silently drained.
		if r.consuming.CompareAndSwap(false, true) {
			r.wg.Add(1)
			go func() {
				defer r.wg.Done()
				r.consumeChatModelStream(ctx, output)
			}()
		} else {
			logger.Info("[AgentFlow/Callback] skipping duplicate ChatModel stream (name=%s), draining", info.Name)
			if output != nil {
				go func() {
					for {
						_, err := (*output).Recv()
						if err != nil {
							break
						}
					}
				}()
			}
		}

	case compose.ComponentOfToolsNode:
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.consumeToolsNodeStream(ctx, output)
		}()

	default:
		// Graph-level and other component stream outputs are duplicates of
		// inner node streams. Drain and discard them to avoid resource leaks.
		if output != nil {
			go func() {
				for {
					if _, err := (*output).Recv(); err != nil {
						break
					}
				}
			}()
		}
	}
	return ctx
}

// consumeChatModelStream reads streaming chunks from the ChatModel callback
// output and translates them into TextDelta and ToolCallStart events.
//
// Additionally, it accumulates chunks into a complete assistant message.
// When the stream ends, if the message contains tool_calls, the full
// assistant message is appended to the MessageCollector so it can be
// persisted to session history (preserving tool call context for future turns).
func (r *ReplayChunkCallback) consumeChatModelStream(_ context.Context, output *schema.StreamReader[callbacks.CallbackOutput]) {
	// Reset consuming flag when this goroutine finishes, so the NEXT ReAct
	// round's ChatModel callback can successfully CAS(false→true) and consume.
	// Without this, consuming stays true forever after the first round, and all
	// subsequent rounds' ChatModel streams are drained as "duplicates" — meaning
	// no EventTextDelta events are emitted for later rounds (including the final
	// text reply), causing the CLI to show no output.
	defer r.consuming.Store(false)

	if output == nil {
		return
	}
	sr := schema.StreamReaderWithConvert(output, func(t callbacks.CallbackOutput) (*schema.Message, error) {
		cbOut := model.ConvCallbackOutput(t)
		if cbOut == nil || cbOut.Message == nil {
			return nil, nil
		}
		return cbOut.Message, nil
	})

	// Accumulate chunks to build the complete assistant message for collection.
	var chunks []*schema.Message

	for {
		msg, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			logger.Warn("[AgentFlow/Callback] ChatModel stream error: %v", err)
			break
		}
		if msg == nil {
			continue
		}

		// Apply response-side stream middleware chain (aligned with OpenClaw's
		// wrapStreamTrimToolCallNames + payloadLogger wrapper pipeline).
		msg = r.middleware.Apply(msg)
		if msg == nil {
			continue
		}

		chunks = append(chunks, msg)

		if msg.Content != "" {
			//logger.Info("[AgentFlow/Callback] ChatModel chunk: content_len=%d content=%.100s", len(msg.Content), msg.Content)
			r.sw.Send(&entity.AgentEvent{
				Type:  entity.EventTextDelta,
				Delta: msg.Content,
			}, nil)
		}

		// Stream reasoning/thinking content (supported by Deepseek R1, Claude, Gemini, Qwen)
		if msg.ReasoningContent != "" {
			r.sw.Send(&entity.AgentEvent{
				Type:           entity.EventReasoningDelta,
				ReasoningDelta: msg.ReasoningContent,
			}, nil)
		}

		for _, tc := range msg.ToolCalls {
			r.sw.Send(&entity.AgentEvent{
				Type: entity.EventToolCallStart,
				ToolCall: &entity.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}, nil)
		}
	}

	// Collect intermediate assistant messages with tool_calls into the
	// MessageCollector. These are needed for session history so the agent
	// remembers what tools it called on the next turn.
	// We only collect messages that contain tool calls — the final assistant
	// text message will be saved separately by runner.go.
	if r.collector != nil && len(chunks) > 0 {
		assembled, err := schema.ConcatMessages(chunks)
		if err == nil && assembled != nil && len(assembled.ToolCalls) > 0 {
			r.collector.Append(assembled)
		}
	}
}

// consumeToolsNodeStream reads tool execution results and emits ToolCallEnd events.
//
// Tool result messages are also appended to the MessageCollector so they are
// persisted in the session history alongside the assistant's tool_call messages.
//
// IMPORTANT: Eino's ToolsNode.Stream returns *schema.StreamReader[[]*schema.Message].
// When the callback system wraps this stream via OnEndWithStreamOutputHandle[T],
// each chunk of type T=[]*schema.Message is directly cast to callbacks.CallbackOutput
// (via `func(i T) (CallbackOutput, error) { return i, nil }`).
//
// Therefore, the actual runtime type of each CallbackOutput chunk received here
// is []*schema.Message (a SLICE of messages), NOT *schema.Message (a single message).
//
// The previous code used model.ConvCallbackOutput(t), which only handles
// *model.CallbackOutput and *schema.Message — it returns nil for []*schema.Message,
// silently dropping ALL tool result messages. This caused session history to contain
// assistant(tool_calls) without subsequent tool(result) messages, triggering
// DeepSeek HTTP 400: "tool_calls must be followed by tool messages".
func (r *ReplayChunkCallback) consumeToolsNodeStream(_ context.Context, output *schema.StreamReader[callbacks.CallbackOutput]) {
	if output == nil {
		return
	}

	var messages []*schema.Message

	for {
		chunk, err := (*output).Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			logger.Warn("[AgentFlow/Callback] ToolsNode stream error: %v", err)
			break
		}
		if chunk == nil {
			continue
		}

		// Eino's ToolsNode produces []*schema.Message per stream chunk (one tool
		// result message per tool call in the batch). We must type-assert to the
		// slice type, not the single-message type.
		switch v := chunk.(type) {
		case []*schema.Message:
			for _, msg := range v {
				if msg != nil {
					messages = append(messages, msg)
				}
			}
		case *schema.Message:
			// Defensive: handle single message in case Eino ever changes behavior.
			if v != nil {
				messages = append(messages, v)
			}
		default:
			logger.Debug("[AgentFlow/Callback] ToolsNode stream: unexpected chunk type %T, skipping", chunk)
		}
	}

	logger.Info("[AgentFlow/Callback] ToolsNode stream collected %d tool result messages", len(messages))

	// Collect tool result messages into the MessageCollector for session persistence.
	if r.collector != nil && len(messages) > 0 {
		r.collector.Append(messages...)
	}

	for _, msg := range messages {
		r.sw.Send(&entity.AgentEvent{
			Type: entity.EventToolCallEnd,
			ToolResult: &entity.ToolResult{
				ToolCallID: msg.ToolCallID,
				Name:       msg.Name,
				Content:    msg.Content,
			},
		}, nil)
	}
}

// OnError intercepts execution errors and emits error events.
//
// IMPORTANT: Eino's callback system fires OnError at EVERY level of the graph
// hierarchy (Tool → ToolsNode → Graph → Lambda → Chain) for the same underlying
// error. If we emit EventError at every level, the stream consumer (e.g.,
// SubAgentManager) sees N duplicate error events for one failure.
//
// Solution: only emit EventError for the innermost component levels
// (ChatModel and ToolsNode). Outer wrappers (Graph, Lambda, Chain) only
// log at debug level since they propagate the same error.
func (r *ReplayChunkCallback) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	switch info.Component {
	case components.ComponentOfChatModel, compose.ComponentOfToolsNode:
		// Inner-most component error — emit to stream.
		logger.Warn("[AgentFlow/Callback] error in %s/%s: %v", info.Component, info.Name, err)
		r.sw.Send(&entity.AgentEvent{
			Type:  entity.EventError,
			Error: err.Error(),
		}, nil)
	default:
		// Outer wrapper (Graph, Lambda, Chain) — the same error is already
		// emitted by the inner handler above. Only log at debug level to
		// avoid duplicate EventError events in the stream.
		logger.Debug("[AgentFlow/Callback] propagated error in %s/%s: %v", info.Component, info.Name, err)
	}
	return ctx
}
