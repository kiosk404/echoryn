package agentflow

import (
	"context"
	"errors"
	"io"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// ReplayChunkCallback is the Eino callbacks.Handler that intercepts streaming
// events from the execution graph and translates them into AgentEvent entities.
// It intercepts:
// - ChatModel stream outputs -> EventTextDelta events
// - ToolsNode start -> EventToolCall events
// - ToolsNode end -> EventToolResult events
// - Errors -> EventError events
// All events are pushed into a schema.StreamWriter[*entity.AgentEvent].
type ReplayChunkCallback struct {
	sw *schema.StreamWriter[*entity.AgentEvent]
}

// NewReplayChunkCallback creates a new ReplayChunkCallback.
func NewReplayChunkCallback(sw *schema.StreamWriter[*entity.AgentEvent]) *ReplayChunkCallback {
	return &ReplayChunkCallback{sw: sw}
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
	switch info.Component {
	case compose.ComponentOfGraph, components.ComponentOfChatModel:
		go r.consumeChatModelStream(ctx, output)

	case compose.ComponentOfToolsNode:
		go r.consumeToolsNodeStream(ctx, output)

	default:
		if output != nil {
			(*output).Close()
		}
	}
	return ctx
}

// consumeChatModelStream reads streaming chunks from the ChatModel callback
// output and translates them into TextDelta and ToolCallStart events.
func (r *ReplayChunkCallback) consumeChatModelStream(_ context.Context, output *schema.StreamReader[callbacks.CallbackOutput]) {
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

		if msg.Content != "" {
			r.sw.Send(&entity.AgentEvent{
				Type:  entity.EventTextDelta,
				Delta: msg.Content,
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
}

// consumeToolsNodeStream reads tool execution results and emits ToolCallEnd events.
func (r *ReplayChunkCallback) consumeToolsNodeStream(_ context.Context, output *schema.StreamReader[callbacks.CallbackOutput]) {
	if output == nil {
		return
	}

	var messages []*schema.Message
	sr := schema.StreamReaderWithConvert(output, func(t callbacks.CallbackOutput) (*schema.Message, error) {
		cbOut := model.ConvCallbackOutput(t)
		if cbOut == nil || cbOut.Message == nil {
			return nil, nil
		}
		return cbOut.Message, nil
	})

	for {
		msg, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			logger.Warn("[AgentFlow/Callback] ToolsNode stream error: %v", err)
			break
		}
		if msg != nil {
			messages = append(messages, msg)
		}
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
func (r *ReplayChunkCallback) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	logger.Warn("[AgentFlow/Callback] error in %s/%s: %v", info.Component, info.Name, err)
	r.sw.Send(&entity.AgentEvent{
		Type:  entity.EventError,
		Error: err.Error(),
	}, nil)
	return ctx
}
