package entity

import (
	"fmt"
)

// ModelInstance represents a concrete, usable model instance registered in the system.
type ModelInstance struct {
	// ID is the unique identifier for the model instance.
	ID int64 `json:"id"`
	// ModelID is the model identifier as registered with the provider.(e.g. "gpt-5.2", "deepseek-v4")
	ModelID string `json:"model_id"`
	// ProviderID references the provider that hosts this model instance.
	ProviderID string `json:"provider_id"`
	// Type classifies the model: LLM, TextEmbedding, Rerank.
	Type ModelType `json:"type"`
	// DisplayInfo contains human-readable info (name, description, token limits.)
	DisplayInfo DisplayInfo `json:"display_info"`
	//Provider    ModelProvider    `json:"provider"`
	// IsDefault indicates whether this is the system default model.
	IsDefault bool `json:"is_default"`
	// Connection holds the resolved connection parameters.
	Connection Connection `json:"connection"`
	// Capability describes what this model can do (e.g. "Video", "Text", "Audio")
	Capability ModelAbility `json:"capability"`
	// Parameters holds model-specific configuration options.
	Parameters []ModelParameter `json:"parameters"`
	// Extra holds additional configuration
	Extra ModelExtra `json:"extra"`
	// Cost defines token pricing for this model.
	Cost ModelCostInfo `json:"cost"`
	// ContextWindow is the maximum number of tokens the model can process in a single request.
	ContextWindow int `json:"context_window"`
	// MaxTokens is the maximum number of tokens the model can generate in a single request.
	MaxTokens int `json:"max_tokens"`
	// Reasoning indicates whether this model supports reasoning tasks.
	Reasoning bool `json:"reasoning"`
	// InputTypes lists the types of input this model can handle (e.g. "Text", "Image", "Audio")
	InputTypes []string `json:"input_types"`
	// Status indicates the current operational state of the model.
	Status ModelStatus `json:"status"`
}

type ModelClass int64

const (
	ModelClass_GPT      ModelClass = 1
	ModelClass_QWen     ModelClass = 2
	ModelClass_Gemini   ModelClass = 3
	ModelClass_DeepSeek ModelClass = 4
	ModelClass_Ollama   ModelClass = 5
	ModelClass_Claude   ModelClass = 6
	ModelClass_Kimi     ModelClass = 7
	ModelClass_GLM      ModelClass = 8
	ModelClass_Other    ModelClass = 999
)

func (p ModelClass) String() string {
	switch p {
	case ModelClass_GPT:
		return "gpt"
	case ModelClass_QWen:
		return "qwen"
	case ModelClass_Gemini:
		return "gemini"
	case ModelClass_DeepSeek:
		return "deepseek"
	case ModelClass_Ollama:
		return "ollama"
	case ModelClass_Claude:
		return "claude"
	case ModelClass_Kimi:
		return "kimi"
	case ModelClass_GLM:
		return "glm"
	case ModelClass_Other:
		return "other"
	}
	return "<UNSET>"
}

// ModelCostInfo defines the cost of using a model(per million tokens)
type ModelCostInfo struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

// ModelStatus indicates the current operational state of the model.
type ModelStatus int32

const (
	ModelStatus_Ready    ModelStatus = 0
	ModelStatus_Disabled ModelStatus = 1
	ModelStatus_Error    ModelStatus = 2
	ModelStatus_CoolDown ModelStatus = 3
)

func (s ModelStatus) String() string {
	switch s {
	case ModelStatus_Ready:
		return "Ready"
	case ModelStatus_Disabled:
		return "Disable"
	case ModelStatus_Error:
		return "Error"
	case ModelStatus_CoolDown:
		return "Cooldown"
	default:
		return "Unknown"
	}
}

// ModelRef is a reference to a model instance.
type ModelRef struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

func (r ModelRef) String() string {
	return fmt.Sprintf("%s/%s", r.ProviderID, r.ModelID)
}

func ModelClassFromString(s string) ModelClass {
	switch s {
	case "gpt", "openai":
		return ModelClass_GPT
	case "qwen", "qwen-portal":
		return ModelClass_QWen
	case "gemini", "google":
		return ModelClass_Gemini
	case "deepseek":
		return ModelClass_DeepSeek
	case "ollama":
		return ModelClass_Ollama
	case "claude":
		return ModelClass_Claude
	case "kimi", "moonshot":
		return ModelClass_Kimi
	case "glm", "zhipu":
		return ModelClass_GLM
	}
	return ModelClass_Other
}
