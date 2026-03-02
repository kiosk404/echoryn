package entity

type LLMParams struct {
	Temperature      *float32            `json:"temperature,omitempty"`
	FrequencyPenalty float32             `json:"frequency_penalty,omitempty"`
	PresencePenalty  float32             `json:"presence_penalty,omitempty"`
	MaxTokens        int                 `json:"max_tokens,omitempty"`
	TopP             *float32            `json:"top_p,omitempty"`
	TopK             *int32              `json:"top_k,omitempty"`
	ResponseFormat   ModelResponseFormat `json:"response_format"`
	EnableThinking   *bool               `json:"enable_thinking,omitempty"`

	// ThinkingLevel controls the model's reasoning depth.
	// When set, it overrides Enable Thinking with a more fine-grained control.
	// See ThinkingLevel type for the full level hierarchy.
	ThinkingLevel ThinkingLevel `json:"thinking_level,omitempty"`
}

// ModelResponseFormat defines the format of the model's response.
type ModelResponseFormat int64

const (
	ModelResponseFormatText ModelResponseFormat = iota
	ModelResponseFormatJSON
	ModelResponseFormatMarkdown
)

func (f ModelResponseFormat) String() string {
	switch f {
	case ModelResponseFormatText:
		return "text"
	case ModelResponseFormatJSON:
		return "json"
	case ModelResponseFormatMarkdown:
		return "markdown"
	default:
		return "text"
	}
}
