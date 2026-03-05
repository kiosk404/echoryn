package toolloop

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// GuardedTool wraps an Eino tool.InvokableTool with loop detection.
// Before each invocation, it checks the Detector; if a critical loop is
// detected, the call is blocked and an error returned to the LLM.
//
// This is the Go equivalent of OpenClaw's wrapToolWithBeforeToolCallHook.
type GuardedTool struct {
	inner    tool.InvokableTool
	detector *Detector
}

var _ tool.InvokableTool = (*GuardedTool)(nil)

// WrapTools wraps a list of Eino tools with loop detection guards.
// All tools share the same Detector instance (per-run state).
func WrapTools(tools []tool.BaseTool, detector *Detector) []tool.BaseTool {
	if detector == nil || !detector.cfg.Enabled {
		return tools
	}

	wrapped := make([]tool.BaseTool, len(tools))
	for i, t := range tools {
		if inv, ok := t.(tool.InvokableTool); ok {
			wrapped[i] = &GuardedTool{inner: inv, detector: detector}
		} else {
			// Non-invokable tools (e.g., streamable) are passed through as-is.
			wrapped[i] = t
		}
	}
	return wrapped
}

// Info delegates to the inner tool.
func (g *GuardedTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return g.inner.Info(ctx)
}

// InvokableRun checks for loops, then delegates to the inner tool.
//
// On critical detection: returns an error message (not a Go error) so the LLM
// sees it as a tool result and can adjust its behavior. This matches OpenClaw's
// approach of throwing an error that becomes the tool result.
func (g *GuardedTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	info, _ := g.inner.Info(ctx)
	toolName := ""
	if info != nil {
		toolName = info.Name
	}

	// Pre-call check.
	result := g.detector.Check(toolName, argumentsInJSON)
	if result.Stuck {
		if result.Level == LevelCritical {
			logger.Warn("[ToolLoopGuard] BLOCKED %q: %s", toolName, result.Message)
			// Return as an error to make Eino propagate it up and stop the agent.
			return "", fmt.Errorf("[ToolLoopBlocked] %s", result.Message)
		}
		// Warning level: log but continue execution.
		logger.Warn("[ToolLoopGuard] WARNING %q: %s", toolName, result.Message)
	}

	// Record the call.
	g.detector.Record(toolName, argumentsInJSON)

	// Execute.
	output, err := g.inner.InvokableRun(ctx, argumentsInJSON, opts...)

	// Record outcome: any non-error result is considered "progress".
	// A more sophisticated version could hash outputs and compare.
	hasProgress := err == nil && output != ""
	g.detector.RecordOutcome(hasProgress)

	return output, err
}
