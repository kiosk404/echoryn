package entity

import "encoding/json"

// LLMTaskRequest represents the input parameters for an llm-task invocation.
type LLMTaskRequest struct {
	// Prompt is the system/instruction prompt for the LLM (required).
	Prompt string `json:"prompt"`

	// Input is an optional structured input payload passed to the LLM.
	Input interface{} `json:"input,omitempty"`

	// Schema is an optional JSON Schema to validate the LLM's output.
	Schema json.RawMessage `json:"schema,omitempty"`

	// Provider overrides the default LLM provider.
	Provider string `json:"provider,omitempty"`

	// Model overrides the default model.
	Model string `json:"model,omitempty"`

	// Temperature controls randomness (0.0 to 2.0).
	Temperature *float64 `json:"temperature,omitempty"`

	// MaxTokens overrides the default max output tokens.
	MaxTokens int `json:"max_tokens,omitempty"`

	// TimeoutMs overrides the default timeout in milliseconds.
	TimeoutMs int64 `json:"timeout_ms,omitempty"`
}

// LLMTaskResult is the output of an llm-task invocation.
type LLMTaskResult struct {
	// JSON is the parsed JSON output from the LLM.
	JSON interface{} `json:"json"`

	// Raw is the raw text output before JSON parsing.
	Raw string `json:"raw,omitempty"`

	// Provider is the provider that was used.
	Provider string `json:"provider"`

	// Model is the model that was used.
	Model string `json:"model"`

	// InputTokens is the number of input tokens consumed.
	InputTokens int64 `json:"input_tokens,omitempty"`

	// OutputTokens is the number of output tokens generated.
	OutputTokens int64 `json:"output_tokens,omitempty"`

	// DurationMs is the wall-clock duration in milliseconds.
	DurationMs int64 `json:"duration_ms"`
}
