package runtime

import (
	"context"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg"
	llmEntity "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	llmService "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/service"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// ContextWindowGuard resolves and validates the effective context window size.
//
// This is the Echoryn equivalent of OpenClaw's context-window-guard.ts:
// - Resolves context window from model metadata (via LLM Module)
// - Enforces a hard minimum (16K tokens)
// - Warns on small context windows (e.g., < 4K tokens)
// - Provides the effective window for downstream pruning/compaction decisions
//
// Resolution priority:
// 1. Model's ContextWindow from ModelInstance metadata
// 2. Configured default from module config
// 3. Hardcoded fallback (200,000 -> Claude Opus 4.5 level)
type ContextWindowGuard struct {
	modelManager  llmService.ModelManager
	defaultWindow int
}

const (
	// HardMinimumContextWindow is the minimum context window size allowed.
	// Below this, agent execution should not process.
	HardMinimumContextWindow = 16_000

	// WarnContextWindow is the context window size at which warnings should be issued.
	WarnContextWindow = 32_000

	// DefaultContextWindow is the default context window size used when no other
	// configuration is available.
	DefaultContextWindow = 200_000
)

// NewContextWindowGuard creates a new ContextWindowGuard.
func NewContextWindowGuard(modelManager llmService.ModelManager, defaultWindow int) *ContextWindowGuard {
	if defaultWindow <= 0 {
		defaultWindow = DefaultContextWindow
	}
	return &ContextWindowGuard{
		modelManager:  modelManager,
		defaultWindow: defaultWindow,
	}
}

// ContextWindowInfo holds the resolved context window parameters.
type ContextWindowInfo struct {
	// WindowSize is the total context window in tokens.
	WindowSize int

	// ReserveTokens is the number of tokens to reserve for system prompts,
	// function calls, and other overhead.
	ReserveTokens int

	// UsableTokens is the number of tokens available for actual agent input/output.
	UsableTokens int
}

// Resolve determines the effective context window size for the given model reference.
func (g *ContextWindowGuard) Resolve(ctx context.Context, ref llmEntity.ModelRef) ContextWindowInfo {
	windowSize := g.defaultWindow
	reserveTokens := 4096

	if g.modelManager != nil {
		model, err := g.modelManager.GetModelByRef(ctx, ref)
		if err == nil && model != nil {
			if model.ContextWindow > 0 {
				windowSize = model.ContextWindow
			}
			if model.MaxTokens > 0 {
				reserveTokens = model.MaxTokens
			}
		} else if err != nil {
			logger.WarnX(pkg.ModuleName, "[ContextWindowGuard] failed to resolve model context window, err: %v", "modelRef",
				ref, "err", err)
		}
	}
	if windowSize < HardMinimumContextWindow {
		logger.WarnX(pkg.ModuleName, "[ContextWindowGuard] resolved window size %d is below hard minimum %d, using default %d",
			"windowSize", windowSize, "hardMinimum", HardMinimumContextWindow, "defaultWindow", g.defaultWindow)
		windowSize = HardMinimumContextWindow
	} else if windowSize < WarnContextWindow {
		logger.WarnX(pkg.ModuleName, "[ContextWindowGuard] resolved window size %d is below warn threshold %d, using default %d",
			"windowSize", windowSize, "warnThreshold", WarnContextWindow, "defaultWindow", g.defaultWindow)
	}

	// Ensure reserve doesn't execute more than half the window.
	if reserveTokens > windowSize/2 {
		reserveTokens = windowSize / 2
	}

	logger.DebugX(pkg.ModuleName, "[ContextWindowGuard] resolved window size %d, reserve tokens %d, usable tokens %d",
		"windowSize", windowSize, "reserveTokens", reserveTokens, "usableTokens", windowSize-reserveTokens)

	return ContextWindowInfo{
		WindowSize:    windowSize,
		ReserveTokens: reserveTokens,
		UsableTokens:  windowSize - reserveTokens,
	}
}
