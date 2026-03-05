package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/agentflow"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/toolloop"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	llmEntity "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	llmService "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/service"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// Retry loop constants aligned with OpenClaw's pi-embedded-runner/run.ts.
const (
	// maxOverflowCompactionAttempts is the max number of compaction retries
	// on context overflow before giving up.
	// OpenClaw: MAX_OVERFLOW_COMPACTION_ATTEMPTS = 3.
	maxOverflowCompactionAttempts = 3

	// transientHTTPRetryDelay is the delay before retrying on transient HTTP errors.
	// OpenClaw: TRANSIENT_HTTP_RETRY_DELAY_MS = 2500.
	transientHTTPRetryDelay = 2500 * time.Millisecond
)

// TurnExecutor handles a single agent turn with retry and model fallback.
//
// The execution loop (modeled after OpenClaw's 3-layer architecture):
//
//	Layer 0 (transient HTTP): retry once on transient network errors (2.5s delay)
//	Layer 1 (model fallback): RunWithFallback tries each candidate model
//	Layer 2 (recovery loop):  context overflow → compaction (×3)
//	                           thinking level unsupported → level degradation
//	                           format errors → break (non-recoverable)
//
// The maxRetries field controls the outer recovery loop budget.
// OpenClaw calculates this dynamically: BASE(24) + max(1, profileCount)*8, clamped to [32, 160].
// Since Echoryn doesn't have auth profile rotation yet, a static default suffices.
//
// Stream wrapping pipeline (aligned with OpenClaw's 5-layer wrapper):
//   - Request-side: SanitizerPipeline transforms messages before LLM call
//   - Response-side: StreamMiddlewareChain processes chunks during streaming
type TurnExecutor struct {
	flowBuilder    *agentflow.AgentFlowBuilder
	fallbackExec   *llmService.FallbackExecutor
	contextBuilder *ContextBuilder
	maxRetries     int

	// sanitizers is the request-side message preprocessing pipeline.
	// Applied before each LLM call to clean/normalize the message array.
	// Aligned with OpenClaw's dropThinkingBlocks + sanitizeToolCallIds + trimToolCallNames.
	sanitizers *agentflow.SanitizerPipeline

	// streamMiddleware is the response-side chunk processing chain.
	// Applied to each streaming chunk before it is translated into AgentEvent.
	// Aligned with OpenClaw's wrapStreamTrimToolCallNames + payloadLogger.
	streamMiddleware *agentflow.StreamMiddlewareChain
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
		flowBuilder:      flowBuilder,
		fallbackExec:     fallbackExec,
		contextBuilder:   contextBuilder,
		maxRetries:       maxRetries,
		sanitizers:       agentflow.NewDefaultSanitizerPipeline(),
		streamMiddleware: agentflow.NewDefaultStreamMiddlewareChain(),
	}
}

// WithSanitizers replaces the request-side message sanitizer pipeline.
func (te *TurnExecutor) WithSanitizers(p *agentflow.SanitizerPipeline) *TurnExecutor {
	te.sanitizers = p
	return te
}

// WithStreamMiddleware replaces the response-side stream middleware chain.
func (te *TurnExecutor) WithStreamMiddleware(c *agentflow.StreamMiddlewareChain) *TurnExecutor {
	te.streamMiddleware = c
	return te
}

// TurnRequest contains all inputs for a single turn execution.
type TurnRequest struct {
	Agent    *entity.Agent
	Messages []*schema.Message
	Tools    []tool.BaseTool

	// LoopDetector is the per-run tool loop detector (circuit-breaker).
	// When non-nil, tools are wrapped with loop detection guards.
	// Aligned with OpenClaw's tool-loop-detection.ts
	LoopDetector *toolloop.Detector

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
// The recovery loop handles multiple error types:
//
//  1. Context overflow → compaction (up to 3 times, aligned with OpenClaw)
//  2. Transient HTTP errors → single retry after 2.5s delay
//  3. Thinking level unsupported → degrade to a lower level and retry
//  4. Non-recoverable errors (format, etc.) → fail immediately
//
// Priority chain for ThinkingLevel:
//
//	per-message directive (future) > session override > agent default > off
func (te *TurnExecutor) Execute(
	ctx context.Context,
	req *TurnRequest,
	abort *AbortController,
) (*TurnResult, error) {
	params := req.Agent.LLMParams()

	// Resolve ThinkingLevel through the priority chain:
	//   per-message directive (future) > session override > agent default > off
	// Aligned with OpenClaw's resolvedThinkLevel priority.
	if req.Session != nil && req.Session.ThinkingLevel != "" {
		params.ThinkingLevel = req.Session.ThinkingLevel
	}

	// Recovery state trackers (aligned with OpenClaw's run.ts local variables).
	overflowCompactionAttempts := 0
	transientHTTPRetried := false
	attemptedThinkingLevels := make(map[llmEntity.ThinkingLevel]struct{})

	// Track the initial thinking level for reset on model switch.
	initialThinkingLevel := params.ThinkingLevel
	if initialThinkingLevel != "" {
		attemptedThinkingLevels[initialThinkingLevel] = struct{}{}
	}

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
			func(fa llmEntity.FallbackAttempt, attemptNum, total int) {
				req.EventWriter.Send(&entity.AgentEvent{
					Type:  entity.EventError,
					Error: fmt.Sprintf("model %s failed (attempt %d/%d): %s", fa.Ref, attemptNum, total, fa.Error),
				}, nil)
			},
		)

		if result.OK {
			result.Value.ModelRef = result.Ref
			return result.Value, nil
		}

		combinedErr := result.AllFailedError()

		// --- Recovery branch 1: Context overflow → compaction ---
		// OpenClaw: up to MAX_OVERFLOW_COMPACTION_ATTEMPTS=3
		if isContextOverflowError(combinedErr) && !abort.IsAborted() {
			if overflowCompactionAttempts < maxOverflowCompactionAttempts {
				if compacted := te.tryCompaction(abort, req, params, combinedErr, overflowCompactionAttempts+1); compacted {
					overflowCompactionAttempts++
					continue
				}
			}
			// All compaction attempts exhausted.
			return nil, fmt.Errorf("context overflow after %d compaction attempts: %w",
				overflowCompactionAttempts, combinedErr)
		}

		// --- Recovery branch 2: Thinking level unsupported → degrade ---
		// OpenClaw: pickFallbackThinkingLevel() parses "supported values are: ..." from API error.
		if params.ThinkingLevel.IsEnabled() && isThinkingLevelError(combinedErr) && !abort.IsAborted() {
			fallbackLevel := pickFallbackThinkingLevel(combinedErr, attemptedThinkingLevels)
			if fallbackLevel != "" {
				logger.Info("[TurnExecutor] thinking level %q unsupported, degrading to %q",
					params.ThinkingLevel, fallbackLevel)
				params.ThinkingLevel = fallbackLevel
				attemptedThinkingLevels[fallbackLevel] = struct{}{}

				req.EventWriter.Send(&entity.AgentEvent{
					Type:  entity.EventRunStatus,
					Error: fmt.Sprintf("thinking level degraded to %q, retrying...", fallbackLevel),
				}, nil)
				continue
			}
		}

		// --- Recovery branch 3: Transient HTTP error → single retry with delay ---
		// OpenClaw: TRANSIENT_HTTP_RETRY_DELAY_MS = 2500, retry once.
		if isTransientHTTPError(combinedErr) && !transientHTTPRetried && !abort.IsAborted() {
			transientHTTPRetried = true
			logger.Info("[TurnExecutor] transient HTTP error on attempt %d, retrying after %v...",
				attempt+1, transientHTTPRetryDelay)

			req.EventWriter.Send(&entity.AgentEvent{
				Type:  entity.EventRunStatus,
				Error: "transient error, retrying...",
			}, nil)

			select {
			case <-time.After(transientHTTPRetryDelay):
				// Reset thinking level on transient retry (OpenClaw: advanceAuthProfile resets thinking).
				params.ThinkingLevel = initialThinkingLevel
				attemptedThinkingLevels = make(map[llmEntity.ThinkingLevel]struct{})
				if initialThinkingLevel != "" {
					attemptedThinkingLevels[initialThinkingLevel] = struct{}{}
				}
				continue
			case <-abort.Context().Done():
				return nil, errno.ErrAborted
			}
		}

		// Non-recoverable error.
		return nil, fmt.Errorf("all model candidates exhausted: %w", combinedErr)
	}

	return nil, fmt.Errorf("max retries (%d) exceeded", te.maxRetries)
}

// tryCompaction attempts context compaction and rebuilds messages on success.
// Returns true if compaction succeeded and the caller should retry.
func (te *TurnExecutor) tryCompaction(
	abort *AbortController,
	req *TurnRequest,
	params *llmEntity.LLMParams,
	originalErr error,
	attemptNum int,
) bool {
	if req.Compactor == nil || req.Session == nil {
		logger.Warn("[TurnExecutor] context overflow, compaction not available")
		return false
	}

	logger.Info("[TurnExecutor] context overflow, running compaction (attempt %d/%d)...",
		attemptNum, maxOverflowCompactionAttempts)

	// Get a ChatModel for compaction (use fallback to get the first available).
	compactModel, _, err := te.fallbackExec.GetChatModelWithFallback(
		abort.Context(), req.Agent.Fallback, params)
	if err != nil {
		logger.Warn("[TurnExecutor] failed to get model for compaction: %v", err)
		return false
	}

	_, compactErr := req.Compactor.Compact(abort.Context(), req.Session, compactModel, req.WindowInfo, req.Agent)
	if compactErr != nil {
		logger.Warn("[TurnExecutor] compaction failed (attempt %d/%d): %v",
			attemptNum, maxOverflowCompactionAttempts, compactErr)
		return false
	}

	// Rebuild context with compacted session.
	newBuild := te.contextBuilder.Build(
		req.Agent, req.Session, "", nil, req.WindowInfo,
	)
	req.Messages = newBuild.Messages

	req.EventWriter.Send(&entity.AgentEvent{
		Type:  entity.EventRunStatus,
		Error: fmt.Sprintf("context compacted (attempt %d/%d), retrying...", attemptNum, maxOverflowCompactionAttempts),
	}, nil)

	logger.Info("[TurnExecutor] compaction succeeded (attempt %d/%d), retrying with ~%d tokens",
		attemptNum, maxOverflowCompactionAttempts, newBuild.EstimatedTokens)
	return true
}

// executeSingleAttempt runs the AgentFlow with a specific ChatModel.
//
// Before calling the LLM, messages are run through the SanitizerPipeline
// (request-side: dropThinkingBlocks, sanitizeToolCallIds, trimToolCallNames).
// During streaming, chunks are processed by the StreamMiddlewareChain
// (response-side: trimToolCallNames, chunkLogger).
func (te *TurnExecutor) executeSingleAttempt(
	ctx context.Context,
	req *TurnRequest,
	cm einoModel.BaseChatModel,
) (*TurnResult, error) {
	// Request-side: apply message sanitizer pipeline before sending to LLM.
	// Aligned with OpenClaw's dropThinkingBlocks + sanitizeToolCallIds + trimNames.
	messages := te.sanitizers.Apply(req.Messages)

	runnable, err := te.flowBuilder.Build(ctx, req.Agent, cm, req.Tools, req.LoopDetector)
	if err != nil {
		return nil, fmt.Errorf("failed to build agent flow: %w", err)
	}

	// Response-side: attach stream middleware chain to the callback.
	clb := agentflow.NewReplayChunkCallback(req.EventWriter).
		WithMiddleware(te.streamMiddleware)

	sr, err := runnable.Stream(ctx, messages,
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

// ---------------------------------------------------------------------------
// Error classification helpers
// ---------------------------------------------------------------------------

// isContextOverflowError checks if an error indicates context window overflow.
//
// Aligned with OpenClaw's isContextOverflowError (pi-embedded-helpers/errors.ts)
// which matches 10+ patterns across OpenAI, Anthropic, Gemini, Qwen, DeepSeek, etc.
func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errno.ErrContextOverflow) {
		return true
	}
	errMsg := strings.ToLower(err.Error())
	// OpenAI / generic patterns.
	if strings.Contains(errMsg, "context_length_exceeded") ||
		strings.Contains(errMsg, "maximum context length") ||
		strings.Contains(errMsg, "too many tokens") ||
		strings.Contains(errMsg, "request_too_large") ||
		strings.Contains(errMsg, "exceeds model context window") ||
		strings.Contains(errMsg, "413 request entity too large") {
		return true
	}
	// Anthropic patterns.
	if strings.Contains(errMsg, "prompt is too long") ||
		strings.Contains(errMsg, "request too large") {
		return true
	}
	// Chinese error messages (Qwen, DeepSeek, GLM).
	if strings.Contains(errMsg, "上下文过长") ||
		strings.Contains(errMsg, "上下文超出") ||
		strings.Contains(errMsg, "超出模型上下文窗口") {
		return true
	}
	// Token count patterns.
	if strings.Contains(errMsg, "resulted in") && strings.Contains(errMsg, "tokens") {
		return true
	}
	return false
}

// isTransientHTTPError checks if an error is a transient HTTP/network error
// that might succeed on retry.
//
// Aligned with OpenClaw's isTransientHttpError (agent-runner-execution.ts):
// covers connection reset, refused, timeout, 502/503/504, socket hang up, etc.
func isTransientHTTPError(err error) bool {
	if err == nil {
		return false
	}
	// Don't retry context overflow (handled separately) or format errors.
	if isContextOverflowError(err) {
		return false
	}

	errMsg := strings.ToLower(err.Error())
	transientPatterns := []string{
		"connection reset",
		"connection refused",
		"socket hang up",
		"econnreset",
		"econnrefused",
		"econnaborted",
		"etimedout",
		"esockettimedout",
		"bad gateway",         // 502
		"service unavailable", // 503
		"gateway timeout",     // 504
		"network error",
		"fetch failed",
		"dns resolution failed",
	}
	for _, p := range transientPatterns {
		if strings.Contains(errMsg, p) {
			return true
		}
	}
	return false
}

// isThinkingLevelError checks if an error indicates that the requested thinking level
// is not supported by the model/provider.
//
// Aligned with OpenClaw's pickFallbackThinkingLevel which parses
// "supported values are: ..." from API error responses.
func isThinkingLevelError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "reasoning_effort") ||
		strings.Contains(errMsg, "thinking") && strings.Contains(errMsg, "not supported") ||
		strings.Contains(errMsg, "supported values are") ||
		strings.Contains(errMsg, "invalid.*reasoning") ||
		strings.Contains(errMsg, "thinking_budget") ||
		strings.Contains(errMsg, "enable_thinking")
}

// supportedValuesRegex extracts values from "supported values are: low, medium, high" patterns.
var supportedValuesRegex = regexp.MustCompile(`supported values are[:\s]+([a-zA-Z_,\s]+)`)

// pickFallbackThinkingLevel determines a fallback thinking level from an API error message.
//
// Modeled after OpenClaw's pickFallbackThinkingLevel (pi-embedded-helpers/thinking.ts):
//  1. Parse "supported values are: low, medium, high" from the error
//  2. Try each supported value, skipping already-attempted levels
//  3. Return the first untried level, or "" if none available
//
// If no supported values can be parsed, falls back to systematic degradation:
// xhigh→high→medium→low→minimal→off.
func pickFallbackThinkingLevel(
	err error,
	attempted map[llmEntity.ThinkingLevel]struct{},
) llmEntity.ThinkingLevel {
	if err == nil {
		return ""
	}

	errMsg := err.Error()

	// Try to extract supported values from the error message.
	if matches := supportedValuesRegex.FindStringSubmatch(errMsg); len(matches) > 1 {
		values := strings.Split(matches[1], ",")
		for _, v := range values {
			normalized, normErr := llmEntity.NormalizeThinkingLevel(strings.TrimSpace(v))
			if normErr != nil {
				continue
			}
			if _, tried := attempted[normalized]; !tried {
				return normalized
			}
		}
	}

	// Fallback: systematic degradation from current level downward.
	levels := llmEntity.AllThinkingLevels()
	// Walk from highest to lowest, find the first untried level that's enabled.
	for i := len(levels) - 1; i >= 0; i-- {
		level := levels[i]
		if level == llmEntity.ThinkingLevelOff {
			continue // Don't degrade to off, just fail.
		}
		if _, tried := attempted[level]; !tried {
			return level
		}
	}

	return "" // All levels exhausted.
}
