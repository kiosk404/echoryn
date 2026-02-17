package entity

import (
	"fmt"
)

type ModelProvider struct {
	ID          string            `json:"id"`
	Name        *I18nText         `json:"name"`
	BaseURL     string            `json:"base_url"`
	Description *I18nText         `json:"description"`
	ModelClass  ModelClass        `json:"model_class"`
	APIKey      string            `json:"api_key"`
	API         ModelAPI          `json:"api"`
	AuthHeader  bool              `json:"auth_header"`
	Headers     map[string]string `json:"headers"`
	Enabled     bool              `json:"enabled"`
}

type ModelAPI string

const (
	ModelAPI_OpenAICompletions  ModelAPI = "openai-completions"
	ModelAPI_OpenAIResponses    ModelAPI = "openai-responses"
	ModelAPI_AnthropicMessages  ModelAPI = "anthropic-messages"
	ModelAPI_GoogleGenerativeAI ModelAPI = "google-generative-ai"
	ModelAPI_OllamaGenerative   ModelAPI = "ollama-generate"
)

func (a ModelAPI) String() string {
	return string(a)
}

func ModelAPIFromString(s string) (ModelAPI, error) {
	switch ModelAPI(s) {
	case ModelAPI_AnthropicMessages, ModelAPI_OpenAICompletions, ModelAPI_OpenAIResponses,
		ModelAPI_OllamaGenerative, ModelAPI_GoogleGenerativeAI:
		return ModelAPI(s), nil
	}
	if s == "" {
		return ModelAPI_OpenAICompletions, nil
	}
	return "", fmt.Errorf("unknown model API: %q", s)
}
