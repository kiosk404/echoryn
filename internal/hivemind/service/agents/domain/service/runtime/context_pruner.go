package runtime

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// ContextPruner performs in-memory pruning of messages to fit within
// a context window budget. It does NOT modify the persisted session
// history — only the copy sent to the LLM.
//
// This is the Eidolon equivalent of OpenClaw's context-pruning/pruner.ts,
// implementing a two-stage strategy:
//
//  1. Soft-trim: Truncate large tool results to head+tail with "..." in between.
//     Applied when context usage exceeds softTrimRatio of the window.
//
//  2. Hard-clear: Replace entire tool results with a placeholder.
//     Applied when context usage exceeds hardClearRatio of the window.
//
// Protected messages (last N assistant messages) are never pruned.
type ContextPruner struct {
	estimator *TokenEstimator
	config    PrunerConfig
}

// PrunerConfig holds tunable parameters for context pruning.
type PrunerConfig struct {
	// SoftTrimRatio: when (estimated tokens / usable window) > this, start soft-trimming.
	// Default: 0.3 (matches OpenClaw).
	SoftTrimRatio float64

	// HardClearRatio: when ratio > this, start hard-clearing.
	// Default: 0.5 (matches OpenClaw).
	HardClearRatio float64

	// SoftTrimHeadChars: chars to keep at the start of tool results in soft-trim.
	// Default: 1500.
	SoftTrimHeadChars int

	// SoftTrimTailChars: chars to keep at the end of tool results in soft-trim.
	// Default: 1500.
	SoftTrimTailChars int

	// KeepLastAssistants: number of recent assistant messages to protect from pruning.
	// Default: 3.
	KeepLastAssistants int
}

// DefaultPrunerConfig returns the default pruning configuration (aligned with OpenClaw).
func DefaultPrunerConfig() PrunerConfig {
	return PrunerConfig{
		SoftTrimRatio:      0.3,
		HardClearRatio:     0.5,
		SoftTrimHeadChars:  1500,
		SoftTrimTailChars:  1500,
		KeepLastAssistants: 3,
	}
}

// NewContextPruner creates a new pruner.
func NewContextPruner(estimator *TokenEstimator, config PrunerConfig) *ContextPruner {
	if config.SoftTrimRatio <= 0 {
		config.SoftTrimRatio = 0.3
	}
	if config.HardClearRatio <= 0 {
		config.HardClearRatio = 0.5
	}
	if config.SoftTrimHeadChars <= 0 {
		config.SoftTrimHeadChars = 1500
	}
	if config.SoftTrimTailChars <= 0 {
		config.SoftTrimTailChars = 1500
	}
	if config.KeepLastAssistants <= 0 {
		config.KeepLastAssistants = 3
	}
	return &ContextPruner{
		estimator: estimator,
		config:    config,
	}
}

// PruneResult holds the outcome of a pruning pass.
type PruneResult struct {
	Messages        []*schema.Message
	EstimatedTokens int
	SoftTrimmed     int
	HardCleared     int
}

// Prune applies pruning to fit within usableTokens.
// The returned messages are copies — the originals are not modified.
//
// The pruning strategy:
//  1. Estimate total tokens
//  2. If ratio > softTrimRatio: soft-trim old tool results
//  3. Re-estimate; if ratio > hardClearRatio: hard-clear old tool results
//  4. Return pruned message copies
func (p *ContextPruner) Prune(messages []*schema.Message, usableTokens int) PruneResult {
	if usableTokens <= 0 || len(messages) == 0 {
		return PruneResult{
			Messages:        messages,
			EstimatedTokens: p.estimator.EstimateMessages(messages),
		}
	}

	estimated := p.estimator.EstimateMessages(messages)
	ratio := float64(estimated) / float64(usableTokens)

	if ratio <= p.config.SoftTrimRatio {
		return PruneResult{
			Messages:        messages,
			EstimatedTokens: estimated,
		}
	}

	// Build a deep copy to avoid mutating originals.
	pruned := p.deepCopyMessages(messages)

	// Determine the protection boundary: protect last N assistant messages.
	protectFrom := p.findProtectionBoundary(pruned)

	result := PruneResult{}

	// Stage 1: Soft-trim.
	if ratio > p.config.SoftTrimRatio {
		result.SoftTrimmed = p.applySoftTrim(pruned, protectFrom)
		estimated = p.estimator.EstimateMessages(pruned)
		ratio = float64(estimated) / float64(usableTokens)
		logger.Debug("[ContextPruner] after soft-trim: %d tokens (ratio=%.2f), trimmed %d messages",
			estimated, ratio, result.SoftTrimmed)
	}

	// Stage 2: Hard-clear.
	if ratio > p.config.HardClearRatio {
		result.HardCleared = p.applyHardClear(pruned, protectFrom)
		estimated = p.estimator.EstimateMessages(pruned)
		logger.Debug("[ContextPruner] after hard-clear: %d tokens (ratio=%.2f), cleared %d messages",
			estimated, float64(estimated)/float64(usableTokens), result.HardCleared)
	}

	result.Messages = pruned
	result.EstimatedTokens = estimated
	return result
}

// applySoftTrim truncates tool-role messages to head+tail with "..." separator.
// Returns the number of messages soft-trimmed.
func (p *ContextPruner) applySoftTrim(messages []*schema.Message, protectFrom int) int {
	trimmed := 0
	maxKeep := p.config.SoftTrimHeadChars + p.config.SoftTrimTailChars

	for i := 0; i < protectFrom; i++ {
		msg := messages[i]
		if msg.Role != schema.Tool {
			continue
		}
		runes := []rune(msg.Content)
		if len(runes) <= maxKeep {
			continue
		}

		head := string(runes[:p.config.SoftTrimHeadChars])
		tail := string(runes[len(runes)-p.config.SoftTrimTailChars:])
		msg.Content = fmt.Sprintf("%s\n\n... [%d characters truncated] ...\n\n%s",
			head, len(runes)-maxKeep, tail)
		trimmed++
	}
	return trimmed
}

// applyHardClear replaces entire tool-role messages with a placeholder.
// Returns the number of messages hard-cleared.
func (p *ContextPruner) applyHardClear(messages []*schema.Message, protectFrom int) int {
	cleared := 0
	for i := 0; i < protectFrom; i++ {
		msg := messages[i]
		if msg.Role != schema.Tool {
			continue
		}
		if strings.HasPrefix(msg.Content, "[Old tool result content cleared]") {
			continue // Already cleared in a previous pass.
		}
		msg.Content = "[Old tool result content cleared]"
		cleared++
	}
	return cleared
}

// findProtectionBoundary returns the index before which messages can be pruned.
// The last KeepLastAssistants assistant messages (and everything after them) are protected.
func (p *ContextPruner) findProtectionBoundary(messages []*schema.Message) int {
	assistantCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == schema.Assistant {
			assistantCount++
			if assistantCount >= p.config.KeepLastAssistants {
				return i
			}
		}
	}
	// If fewer than KeepLastAssistants assistants exist, don't prune anything.
	return 0
}

// deepCopyMessages creates a deep copy of the message slice.
// Only Content is mutable during pruning, so we share ToolCalls by reference.
func (p *ContextPruner) deepCopyMessages(messages []*schema.Message) []*schema.Message {
	result := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		cp := *msg
		result[i] = &cp
	}
	return result
}
