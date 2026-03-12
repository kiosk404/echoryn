package agentflow

import (
	"context"
	"fmt"
	"io"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/toolloop"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// maxGraphSteps is the Eino MaxStep ceiling for the ReAct agent.
//
// BUG FIX: Reduced from 1024 to 50. The previous value was excessively large,
// meaning the ReAct agent could make up to 1024 LLM→tool→result rounds per turn
// before Eino would terminate the graph. Combined with a hung LLM API, this
// could cause the agent to appear frozen for a very long time.
//
// The actual execution boundary is still primarily enforced by:
//   - toolloop.Detector (circuit-breaker after N no-progress calls)
//   - context timeout (RunTimeout in AgentRunner, ExecutionTimeout in subagent)
//
// 50 steps is generous enough for any reasonable tool-calling sequence while
// providing a safety net against runaway loops.
const maxGraphSteps = 50

// AgentFlowBuilder constructs an Eino execution graph for agent execution.
//
// This is the Echoryn equivalent of agentflow/agent_flow_builder.go,
// building compose.Runnable that wires together:
//   - ReAct Agent (when tools are available): handles LLM → tool_call → execute → result loop
//   - Simple ChatModel chain (when no tools): direct LLM generation
//
// The resulting Runnable supports both Invoke and Stream execution.
type AgentFlowBuilder struct{}

func NewAgentFlowBuilder() *AgentFlowBuilder {
	return &AgentFlowBuilder{}
}

// FlowBuildResult is the output of AgentFlowBuilder.Build.
type FlowBuildResult struct {
	// Runnable is the compiled Eino execution graph.
	Runnable compose.Runnable[[]*schema.Message, *schema.Message]

	// SteerModel is the SteerAwareChatModel wrapper (nil when steerCh was nil).
	// After execution, call SteerModel.ConsumedMessages() to retrieve all
	// steer-injected sub-agent results that must be persisted to session history.
	SteerModel *SteerAwareChatModel
}

// Build constructs and compiles an Eino Runnable for the given agent configuration.
//
// When tools are provided, it builds a ReAct Agent that handles
// the LLM → tool_call → execute → result → LLM loop automatically.
// Tool call loops are guarded by the toolloop.Detector (circuit-breaker),
// NOT by Eino's MaxStep which is set to a high ceiling.
// When no tools are provided, it uses a plain ChatModel chain.
//
// If steerCh is non-nil, the ChatModel is wrapped with SteerAwareChatModel
// which drains pending sub-agent announcements before each LLM call.
// This implements the Steer delivery path (aligned with OpenClaw's
// queueEmbeddedPiMessage).
func (b *AgentFlowBuilder) Build(
	ctx context.Context,
	agent *entity.Agent,
	chatModel einoModel.BaseChatModel,
	tools []tool.BaseTool,
	loopDetector *toolloop.Detector,
	steerCh <-chan string,
) (*FlowBuildResult, error) {
	// Wrap ChatModel with steer awareness if a steer channel is provided.
	wrapped := NewSteerAwareChatModel(chatModel, steerCh)

	// Extract the SteerAwareChatModel reference (nil if steerCh was nil).
	var steerModel *SteerAwareChatModel
	if sm, ok := wrapped.(*SteerAwareChatModel); ok {
		steerModel = sm
	}

	var runnable compose.Runnable[[]*schema.Message, *schema.Message]
	var err error
	if len(tools) > 0 {
		runnable, err = b.buildWithTools(ctx, agent, wrapped, tools, loopDetector)
	} else {
		runnable, err = b.buildWithoutTools(ctx, wrapped)
	}
	if err != nil {
		return nil, err
	}

	return &FlowBuildResult{
		Runnable:   runnable,
		SteerModel: steerModel,
	}, nil
}

// buildWithTools creates a ReAct Agent and wraps it as compose.Runnable via Chain + AnyLambda.
//
// Tools are wrapped with toolloop.GuardedTool to detect and block infinite loops.
// Eino's MaxStep is set to a high ceiling (maxGraphSteps); the real execution
// boundary comes from the loop detector (circuit-breaker) and context timeout.
//
// react.Agent directly provides Generate/Stream methods, so we wrap it with
// compose.AnyLambda to get compose.Runnable compatible interface.
func (b *AgentFlowBuilder) buildWithTools(
	ctx context.Context,
	agent *entity.Agent,
	chatModel einoModel.BaseChatModel,
	tools []tool.BaseTool,
	loopDetector *toolloop.Detector,
) (compose.Runnable[[]*schema.Message, *schema.Message], error) {
	tcm, ok := chatModel.(einoModel.ToolCallingChatModel)
	if !ok {
		return nil, errno.ErrModelNotToolCapable
	}

	// Wrap tools with loop detection guard (OpenClaw's wrapToolWithBeforeToolCallHook).
	guardedTools := toolloop.WrapTools(tools, loopDetector)

	reactAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: tcm,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: guardedTools,
		},
		MaxStep: maxGraphSteps,
		// Use a full-stream checker instead of the default firstChunkStreamToolCallChecker.
		// The default checker returns false (no tool call) as soon as it sees a non-empty
		// Content chunk, which breaks models like DeepSeek and Claude that emit text
		// content BEFORE tool_calls in the same response. Our checker consumes the
		// entire stream and checks whether ANY chunk contains ToolCalls.
		StreamToolCallChecker: fullStreamToolCallChecker,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ReAct agent: %w", err)
	}

	chain := compose.NewChain[[]*schema.Message, *schema.Message]()
	agentLambda, err := compose.AnyLambda(reactAgent.Generate, reactAgent.Stream, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent lambda: %w", err)
	}
	chain.AppendLambda(agentLambda)

	runnable, err := chain.Compile(ctx, compose.WithGraphName("echoryn_react_agent"))
	if err != nil {
		return nil, fmt.Errorf("failed to compile ReAct agent chain: %w", err)
	}

	logger.Info("[AgentFlow] built ReAct agent for %q with %d tools, (loop detection=%v, max_graph_steps=%d)",
		agent.ID, len(tools), loopDetector != nil, maxGraphSteps)

	return runnable, nil
}

// buildWithoutTools creates a simple ChatModel chain (no tool loop).
//
// Uses AnyLambda (not InvokableLambda) to provide both Generate and Stream
// implementations. This ensures that when executor calls runnable.Stream(),
// the callback's OnEndWithStreamOutput receives proper streaming chunks
// and can emit EventTextDelta events to the client.
func (b *AgentFlowBuilder) buildWithoutTools(
	ctx context.Context,
	chatModel einoModel.BaseChatModel,
) (compose.Runnable[[]*schema.Message, *schema.Message], error) {
	chain := compose.NewChain[[]*schema.Message, *schema.Message]()

	chatLambda, err := compose.AnyLambda(
		func(ctx context.Context, messages []*schema.Message, opts ...einoModel.Option) (*schema.Message, error) {
			return chatModel.Generate(ctx, messages, opts...)
		},
		func(ctx context.Context, messages []*schema.Message, opts ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
			return chatModel.Stream(ctx, messages, opts...)
		},
		nil, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat lambda: %w", err)
	}
	chain.AppendLambda(chatLambda)

	runnable, err := chain.Compile(ctx, compose.WithGraphName("echoryn_agent_simple"))
	if err != nil {
		return nil, fmt.Errorf("failed to compile simple agent chain: %w", err)
	}

	logger.Info("[AgentFlow] built simple ChatModel agent (no tools)")
	return runnable, nil
}

// fullStreamToolCallChecker consumes the entire model stream to determine
// whether any chunk contains tool calls.
//
// The default Eino firstChunkStreamToolCallChecker only inspects the first
// non-empty content chunk — if it sees text before tool_calls it immediately
// concludes "no tool call" and routes to END, which breaks models like DeepSeek
// and Claude that output text content BEFORE tool_calls in the same turn.
//
// This checker drains the full stream so it correctly detects tool_calls
// regardless of their position in the output.
func fullStreamToolCallChecker(_ context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error) {
	defer sr.Close()

	for {
		msg, err := sr.Recv()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(msg.ToolCalls) > 0 {
			return true, nil
		}
	}
}
