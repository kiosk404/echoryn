package trace

import (
	"context"
	"fmt"
)

// StartAgentRun starts a root span for an agent run.
//
// This creates the top-level trace for an agent execution and sets
// the standard agent-level attributes.
//
// Usage:
//
//	ctx, finish := trace.StartAgentRun(ctx, tracer, agentID, agentName, sessionID, runID)
//	defer finish(trace.SpanStatusOK, "")
func StartAgentRun(
	ctx context.Context,
	tracer *Tracer,
	agentID, agentName, sessionID, runID string,
) (context.Context, func(SpanStatus, string)) {
	if tracer == nil {
		return ctx, func(SpanStatus, string) {}
	}

	ctx, span := tracer.Start(ctx, fmt.Sprintf("agent.run/%s", agentName), SpanKindAgentRun)
	span.SetAttribute(AttrAgentID, agentID)
	span.SetAttribute(AttrAgentName, agentName)
	span.SetAttribute(AttrSessionID, sessionID)
	span.SetAttribute(AttrRunID, runID)

	return ctx, func(status SpanStatus, msg string) {
		tracer.EndSpan(span, status, msg)
	}
}

// StartAgentTurn starts a span for a single conversation turn.
//
// Usage:
//
//	ctx, finish := trace.StartAgentTurn(ctx, tracer, turnNumber)
//	defer finish(trace.SpanStatusOK, "")
func StartAgentTurn(
	ctx context.Context,
	tracer *Tracer,
	turnNumber int,
) (context.Context, func(SpanStatus, string)) {
	if tracer == nil {
		return ctx, func(SpanStatus, string) {}
	}

	ctx, span := tracer.Start(ctx, fmt.Sprintf("agent.turn/%d", turnNumber), SpanKindAgentTurn)
	span.SetAttribute(AttrTurnNumber, turnNumber)

	return ctx, func(status SpanStatus, msg string) {
		tracer.EndSpan(span, status, msg)
	}
}

// StartLLMCall starts a span for a single LLM inference call.
//
// Usage:
//
//	ctx, finish := trace.StartLLMCall(ctx, tracer, "openai", "gpt-4o")
//	defer finish(trace.SpanStatusOK, "")
//	// After call, record usage:
//	trace.RecordLLMUsage(ctx, inputTokens, outputTokens, costUSD)
func StartLLMCall(
	ctx context.Context,
	tracer *Tracer,
	provider, model string,
) (context.Context, func(SpanStatus, string)) {
	if tracer == nil {
		return ctx, func(SpanStatus, string) {}
	}

	ctx, span := tracer.Start(ctx, fmt.Sprintf("llm.call/%s/%s", provider, model), SpanKindLLMCall)
	span.SetAttribute(AttrGenAISystem, provider)
	span.SetAttribute(AttrGenAIRequestModel, model)

	return ctx, func(status SpanStatus, msg string) {
		tracer.EndSpan(span, status, msg)
	}
}

// StartToolCall starts a span for a tool/function invocation.
//
// Usage:
//
//	ctx, finish := trace.StartToolCall(ctx, tracer, toolName, callID)
//	defer finish(trace.SpanStatusOK, "")
func StartToolCall(
	ctx context.Context,
	tracer *Tracer,
	toolName, callID string,
) (context.Context, func(SpanStatus, string)) {
	if tracer == nil {
		return ctx, func(SpanStatus, string) {}
	}

	ctx, span := tracer.Start(ctx, fmt.Sprintf("tool.call/%s", toolName), SpanKindToolCall)
	span.SetAttribute(AttrGenAIToolName, toolName)
	if callID != "" {
		span.SetAttribute(AttrGenAIToolCallID, callID)
	}

	return ctx, func(status SpanStatus, msg string) {
		tracer.EndSpan(span, status, msg)
	}
}

// StartCompaction starts a span for a context compaction operation.
//
// Usage:
//
//	ctx, finish := trace.StartCompaction(ctx, tracer, attemptNum)
//	defer finish(trace.SpanStatusOK, "")
func StartCompaction(
	ctx context.Context,
	tracer *Tracer,
	attemptNum int,
) (context.Context, func(SpanStatus, string)) {
	if tracer == nil {
		return ctx, func(SpanStatus, string) {}
	}

	ctx, span := tracer.Start(ctx, fmt.Sprintf("agent.compaction/%d", attemptNum), SpanKindCompaction)
	span.SetAttribute(AttrCompactionAttempt, attemptNum)

	return ctx, func(status SpanStatus, msg string) {
		tracer.EndSpan(span, status, msg)
	}
}

// RecordLLMUsage adds token usage attributes to the active span.
func RecordLLMUsage(ctx context.Context, inputTokens, outputTokens int64, costUSD float64) {
	span := SpanFromContext(ctx)
	if span == nil {
		return
	}
	span.SetAttribute(AttrGenAIUsageInputTokens, inputTokens)
	span.SetAttribute(AttrGenAIUsageOutputTokens, outputTokens)
	if costUSD > 0 {
		span.SetAttribute(AttrGenAITokenCostUSD, costUSD)
	}
}

// RecordLLMResponse adds response-level attributes to the active span.
func RecordLLMResponse(ctx context.Context, responseModel, responseID string, finishReasons []string) {
	span := SpanFromContext(ctx)
	if span == nil {
		return
	}
	if responseModel != "" {
		span.SetAttribute(AttrGenAIResponseModel, responseModel)
	}
	if responseID != "" {
		span.SetAttribute(AttrGenAIResponseID, responseID)
	}
	if len(finishReasons) > 0 {
		span.SetAttribute(AttrGenAIResponseFinishReasons, finishReasons)
	}
}

// SetSpanError marks the active span as errored with the given message.
func SetSpanError(ctx context.Context, err error) {
	span := SpanFromContext(ctx)
	if span == nil || err == nil {
		return
	}
	span.Status = SpanStatusError
	span.StatusMessage = err.Error()
}

// AddSpanEvent adds an event to the active span.
func AddSpanEvent(ctx context.Context, name string, attrs map[string]interface{}) {
	span := SpanFromContext(ctx)
	if span == nil {
		return
	}
	span.AddEvent(name, attrs)
}
