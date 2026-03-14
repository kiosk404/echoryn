package executor

import (
	"context"
	"io"

	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/subagent"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// LocalExecutor wraps the existing AgentExecutor to implement the Executor interface.
// This is the adapter between the new execution routing layer and the current local
// SubAgent execution pipeline.
type LocalExecutor struct {
	agentExecutor subagent.AgentExecutor
}

// NewLocalExecutor creates a LocalExecutor wrapping the existing AgentExecutor.
func NewLocalExecutor(agentExecutor subagent.AgentExecutor) *LocalExecutor {
	return &LocalExecutor{
		agentExecutor: agentExecutor,
	}
}

func (e *LocalExecutor) Name() string { return "local" }

// Execute starts a Subagent run using the existing local execution pipeline.
func (e *LocalExecutor) Execute(ctx context.Context, req *ExecuteRequest) error {
	// Convert to the existing ExecuteRequest format.
	execReq := &subagent.ExecuteRequest{
		AgentID:   req.AgentID,
		SessionID: req.SessionID,
		Input:     req.Input,
	}

	sr, err := e.agentExecutor.RunSubAgent(ctx, execReq)
	if err != nil {
		return err
	}

	// Consume the stream asynchronously (the existing executor handles result delivery).
	go e.consumeStream(sr)

	return nil
}

// consumeStream drains the event stream to prevent goroutine leaks.
func (e *LocalExecutor) consumeStream(sr *schema.StreamReader[*entity.AgentEvent]) {
	if sr == nil {
		return
	}

	defer sr.Close()
	for {
		_, err := sr.Recv()
		if err != nil {
			if err != io.EOF {
				logger.Warn("[LocalExecutor] stream error: %v", err)
			}
			return
		}
	}
}
