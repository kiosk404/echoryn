// Package thinking provides a Strategy pattern for mapping Echoryn's unified
// ThinkingLevel to provider-specific eino model configurations.
//
// Each provider that supports reasoning/thinking registers a ThinkingStrategy
// implementation. Providers that don't support thinking simply use NoopStrategy.
//
// Architecture (Strategy Pattern):
//
//	ThinkingStrategy (interface)
//	├── OpenAIStrategy      → sets ChatModelConfig.ReasoningEffort
//	├── ClaudeStrategy      → sets Config.Thinking
//	├── GeminiStrategy      → sets Config.ThinkingConfig
//	├── QwenStrategy        → sets Config.EnableThinking
//	├── OllamaStrategy      → sets Config.Thinking (config-level)
//	├── DeepSeekStrategy    → no-op (model-intrinsic reasoning)
//	└── NoopStrategy        → no-op fallback
//
// Usage at build time:
//
//	strategy := thinking.ForProvider(providerName)
//	thinking.ResolveAndApply(strategy, params.ThinkingLevel, &providerConfig)
//
// Usage with runtime model.Option (OpenAI, Claude):
//
//	options := thinking.ResolveOptions(strategy, params.ThinkingLevel)
//	chatModel.Generate(ctx, messages, options...)
package thinking

import (
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
)

// Strategy maps a ThinkingLevel to provider-specific eino model.Option(s).
//
// Each provider implements this interface to translate the unified ThinkingLevel
// into its SDK's native thinking/reasoning API.
//
// Implementations MUST be stateless and goroutine-safe.
type Strategy interface {
	// Name returns the provider name for logging/debugging.
	Name() string

	// MaxLevel returns the highest ThinkingLevel this provider supports.
	// Levels above this are automatically clamped down.
	MaxLevel() entity.ThinkingLevel

	// Apply returns the eino model.Option(s) that configure thinking
	// for the given level. Returns nil if no options are needed.
	//
	// These options can be used either:
	// - At runtime: passed to Generate()/Stream() calls
	// - At build time: the provider's BuildChatModel reads ThinkingLevel from LLMParams
	//   and applies it to the provider config directly
	Apply(level entity.ThinkingLevel) []model.Option
}

// ResolveOptions resolves the final model.Option list for a given thinking level and strategy.
//
// It handles:
//  1. Level clamping (if level exceeds strategy's MaxLevel)
//  2. Delegation to the strategy's Apply method
//  3. Nil-safe (returns nil for Off level or nil strategy)
func ResolveOptions(strategy Strategy, level entity.ThinkingLevel) []model.Option {
	if strategy == nil || !level.IsEnabled() {
		return nil
	}

	// Clamp to provider's max supported level.
	clamped := level.ClampMax(strategy.MaxLevel())
	return strategy.Apply(clamped)
}

// ClampLevel clamps a ThinkingLevel to the provider's maximum supported level.
// Useful when providers handle thinking at config-build time rather than runtime.
func ClampLevel(strategy Strategy, level entity.ThinkingLevel) entity.ThinkingLevel {
	if strategy == nil || !level.IsEnabled() {
		return level
	}
	return level.ClampMax(strategy.MaxLevel())
}
