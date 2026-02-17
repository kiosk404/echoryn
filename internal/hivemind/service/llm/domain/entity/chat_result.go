package entity

// ChatCompletionResult holds the result of a chat completion request.
type ChatCompletionResult struct {
	// Content is the generated text content.
	Content string `json:"content"`

	// ToolCalls contains any tool/function calls made by the model.
	ToolCalls []ToolCallResult `json:"tool_calls,omitempty"`

	// Usage contains token usage statistics.
	Usage *TokenUsage `json:"usage,omitempty"`

	// FinishReason indicates why the model stopped generating tokens.
	FinishReason string `json:"finish_reason,omitempty"`
}

type ToolCallResult struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
