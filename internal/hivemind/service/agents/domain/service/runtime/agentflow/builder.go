package agentflow

import (
	"context"
	"fmt"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// AgentFlowBuilder constructs an Eino execution graph for agent execution.
//
// This is the Eidolon equivalent of agentflow/agent_flow_builder.go,
// building compose.Runnable that wires together:
//   - ReAct Agent (when tools are available): handles LLM → tool_call → execute → result loop
//   - Simple ChatModel chain (when no tools): direct LLM generation
//
// The resulting Runnable supports both Invoke and Stream execution.
type AgentFlowBuilder struct{}

func NewAgentFlowBuilder() *AgentFlowBuilder {
	return &AgentFlowBuilder{}
}

// Build constructs and compiles an Eino Runnable for the given agent configuration.
//
// When tools are provided, it builds a ReAct Agent that handles
// the LLM → tool_call → execute → result → LLM loop automatically.
// When no tools are provided, it uses a plain ChatModel chain.
func (b *AgentFlowBuilder) Build(
	ctx context.Context,
	agent *entity.Agent,
	chatModel einoModel.BaseChatModel,
	tools []tool.BaseTool,
	maxTurns int,
) (compose.Runnable[[]*schema.Message, *schema.Message], error) {
	if len(tools) > 0 {
		return b.buildWithTools(ctx, agent, chatModel, tools, maxTurns)
	}
	return b.buildWithoutTools(ctx, chatModel)
}

// buildWithTools creates a ReAct Agent and wraps it as compose.Runnable via Chain + AnyLambda.
//
// react.Agent directly provides Generate/Stream methods, so we wrap it with
// compose.AnyLambda to get compose.Runnable compatible interface.
func (b *AgentFlowBuilder) buildWithTools(
	ctx context.Context,
	agent *entity.Agent,
	chatModel einoModel.BaseChatModel,
	tools []tool.BaseTool,
	maxTurns int,
) (compose.Runnable[[]*schema.Message, *schema.Message], error) {
	tcm, ok := chatModel.(einoModel.ToolCallingChatModel)
	if !ok {
		return nil, errno.ErrModelNotToolCapable
	}

	reactAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: tcm,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
		MaxStep: maxTurns,
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

	runnable, err := chain.Compile(ctx, compose.WithGraphName("eidolon_react_agent"))
	if err != nil {
		return nil, fmt.Errorf("failed to compile ReAct agent chain: %w", err)
	}

	logger.Info("[AgentFlow] built ReAct agent for %q with %d tools, max_turns=%d",
		agent.ID, len(tools), maxTurns)

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

	runnable, err := chain.Compile(ctx, compose.WithGraphName("eidolon_agent_simple"))
	if err != nil {
		return nil, fmt.Errorf("failed to compile simple agent chain: %w", err)
	}

	logger.Info("[AgentFlow] built simple ChatModel agent (no tools)")
	return runnable, nil
}
