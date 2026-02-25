// Package entity defines configuration and types for the llm-task plugin.
package entity

// LLMTaskConfig is the resolved configuration for the llm-task plugin.
// This corresponds to OpenClaw's llm-task plugin configSchema.
type LLMTaskConfig struct {
	// Enabled controls whether the llm-task plugin is active.
	Enabled bool `json:"enabled"`

	// DefaultProvider is the default LLM provider ID (e.g., "deepseek").
	DefaultProvider string `json:"default_provider"`

	// DefaultModel is the default model ID (e.g., "deepseek-chat").
	DefaultModel string `json:"default_model"`

	// AllowedModels is an optional allowlist of "provider/model" keys.
	// If non-empty, only these models can be used. Empty means all allowed.
	AllowedModels []string `json:"allowed_models,omitempty"`

	// MaxTokens is the default maximum output tokens.
	MaxTokens int `json:"max_tokens"`

	// TimeoutMs is the default timeout for LLM calls in milliseconds.
	TimeoutMs int64 `json:"timeout_ms"`
}

// DefaultLLMTaskConfig returns a sensible default configuration.
func DefaultLLMTaskConfig() *LLMTaskConfig {
	return &LLMTaskConfig{
		Enabled:   true,
		MaxTokens: 4096,
		TimeoutMs: 60000, // 60 seconds
	}
}
