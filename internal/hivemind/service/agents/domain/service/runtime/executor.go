package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/agentflow"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	llmEntity "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	llmService "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/service"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// TurnExecutor handles a single agent turn with retry and model fallback.
//
// The execution loop:
//  1. Build Eino Runnable via AgentFlowBuilder
//  2. Stream execute with Callbacks intercepting events
//  3. On failure: classify error → retry with fallback model or compact context
//  4. On success: collect final message and return
type TurnExecutor struct {
	flowBuilder    *agentflow.AgentFlowBuilder
	fallbackExec   *llmService.FallbackExecutor
	contextBuilder *ContextBuilder
	maxRetries     int
}

// NewTurnExecutor creates a new TurnExecutor.
func NewTurnExecutor(
	flowBuilder *agentflow.AgentFlowBuilder,
	fallbackExec *llmService.FallbackExecutor,
	contextBuilder *ContextBuilder,
	maxRetries int,
) *TurnExecutor {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &TurnExecutor{
		flowBuilder:    flowBuilder,
		fallbackExec:   fallbackExec,
		contextBuilder: contextBuilder,
		maxRetries:     maxRetries,
	}
}

// TurnRequest contains all inputs for a single turn execution.
type TurnRequest struct {
	Agent    *entity.Agent
	Messages []*schema.Message
	Tools    []tool.BaseTool
	MaxTurns int

	EventWriter *schema.StreamWriter[*entity.AgentEvent]

	// Session is needed for compaction on overflow.
	Session *entity.Session

	// WindowInfo is the resolved context window parameters.
	WindowInfo ContextWindowInfo

	// Compactor performs compaction when context overflow is detected.
	// May be nil if compaction is not configured.
	Compactor *Compactor
}

// TurnResult is the output of a successful turn execution.
type TurnResult struct {
	FinalMessage *schema.Message
	ModelRef     llmEntity.ModelRef
	Usage        *entity.TokenUsage
	Compacted    bool
}

// Execute runs a single agent turn with fallback and retry logic.
//
// The flow:
//  1. Use FallbackExecutor to try models in order
//  2. For each model: build AgentFlow → stream execute → collect result
//  3. On context overflow: compact session history, rebuild context, retry
//  4. On abort: return immediately
func (te *TurnExecutor) Execute(
	ctx context.Context,
	req *TurnRequest,
	abort *AbortController,
) (*TurnResult, error) {
	params := req.Agent.LLMParams()
	compactionAttempted := false

	for attempt := 0; attempt < te.maxRetries; attempt++ {
		if err := abort.CheckAborted(); err != nil {
			return nil, err
		}

		result := llmService.RunWithFallback(
			abort.Context(),
			te.fallbackExec,
			req.Agent.Fallback,
			params,
			func(ctx context.Context, cm einoModel.BaseChatModel) (*TurnResult, error) {
				return te.executeSingleAttempt(ctx, req, cm)
			},
			func(attempt llmEntity.FallbackAttempt, attemptNum, total int) {
				req.EventWriter.Send(&entity.AgentEvent{
					Type:  entity.EventError,
					Error: fmt.Sprintf("model %s failed (attempt %d/%d): %s", attempt.Ref, attemptNum, total, attempt.Error),
				}, nil)
			},
		)

		if result.OK {
			result.Value.ModelRef = result.Ref
			return result.Value, nil
		}

		combinedErr := result.AllFailedError()

		// Context overflow → try compaction once, then retry.
		if isContextOverflowError(combinedErr) && !compactionAttempted {
			compactionAttempted = true

			if req.Compactor != nil && req.Session != nil {
				logger.Info("[TurnExecutor] context overflow on attempt %d, running compaction...", attempt+1)

				// Get a ChatModel for compaction (use fallback to get the first available).
				compactModel, _, err := te.fallbackExec.GetChatModelWithFallback(
					abort.Context(), req.Agent.Fallback, params)
				if err != nil {
					logger.Warn("[TurnExecutor] failed to get model for compaction: %v", err)
					return nil, fmt.Errorf("context overflow and compaction model unavailable: %w", combinedErr)
				}

				_, compactErr := req.Compactor.Compact(abort.Context(), req.Session, compactModel, req.WindowInfo)
				if compactErr != nil {
					logger.Warn("[TurnExecutor] compaction failed: %v", compactErr)
					return nil, fmt.Errorf("context overflow and compaction failed: %w", combinedErr)
				}

				// Rebuild context with compacted session.
				newBuild := te.contextBuilder.Build(
					req.Agent, req.Session, "", nil, req.WindowInfo,
				)
				req.Messages = newBuild.Messages

				req.EventWriter.Send(&entity.AgentEvent{
					Type:  entity.EventRunStatus,
					Error: "context compacted, retrying...",
				}, nil)

				logger.Info("[TurnExecutor] compaction succeeded, retrying with %d tokens", newBuild.EstimatedTokens)
				continue
			}

			logger.Warn("[TurnExecutor] context overflow on attempt %d, compaction not available", attempt+1)
			return nil, fmt.Errorf("context overflow (compaction not configured): %w", combinedErr)
		}

		return nil, fmt.Errorf("all model candidates exhausted: %w", combinedErr)
	}

	return nil, fmt.Errorf("max retries (%d) exceeded", te.maxRetries)
}

// executeSingleAttempt runs the AgentFlow with a specific ChatModel.
func (te *TurnExecutor) executeSingleAttempt(
	ctx context.Context,
	req *TurnRequest,
	cm einoModel.BaseChatModel,
) (*TurnResult, error) {
	runnable, err := te.flowBuilder.Build(ctx, req.Agent, cm, req.Tools, req.MaxTurns)
	if err != nil {
		return nil, fmt.Errorf("failed to build agent flow: %w", err)
	}

	clb := agentflow.NewReplayChunkCallback(req.EventWriter)

	sr, err := runnable.Stream(ctx, req.Messages,
		compose.WithCallbacks(clb.Build()),
	)
	if err != nil {
		return nil, fmt.Errorf("agent flow stream failed: %w", err)
	}

	finalMsg, err := collectStreamResult(sr)
	if err != nil {
		return nil, err
	}

	return &TurnResult{
		FinalMessage: finalMsg,
	}, nil
}

// collectStreamResult reads from the stream and concatenates all message chunks.
func collectStreamResult(sr *schema.StreamReader[*schema.Message]) (*schema.Message, error) {
	if sr == nil {
		return nil, fmt.Errorf("nil stream reader")
	}

	var chunks []*schema.Message
	for {
		msg, err := (*sr).Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("stream recv error: %w", err)
		}
		if msg != nil {
			chunks = append(chunks, msg)
		}
	}

	if len(chunks) == 0 {
		return &schema.Message{
			Role:    schema.Assistant,
			Content: "",
		}, nil
	}

	finalMsg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return nil, fmt.Errorf("failed to concat messages: %w", err)
	}

	return finalMsg, nil
}

// isContextOverflowError checks if an error indicates context window overflow.
func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errno.ErrContextOverflow) {
		return true
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "context_length_exceeded") ||
		strings.Contains(errMsg, "maximum context length") ||
		strings.Contains(errMsg, "too many tokens") ||
		strings.Contains(errMsg, "request_too_large") ||
		strings.Contains(errMsg, "exceeds model context window") ||
		strings.Contains(errMsg, "413 request entity too large")
}
