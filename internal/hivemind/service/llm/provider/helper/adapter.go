package helper

import (
	"context"
	"fmt"

	"github.com/bytedance/gg/gptr"
	einoOpenAI "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/thinking"
)

// NewOpenAICompatibleChatModel creates an Eino ChatModel using the OpenAI-compatible API.
// This is the common path for providers that expose an OpenAI-compatible endpoint
// (OpenAI, DeepSeek, Qwen/DashScope, Kimi/Moonshot, GLM/ZhiPu, Ollama, etc.).
func NewOpenAICompatibleChatModel(ctx context.Context, instance *entity.ModelInstance, provider *entity.ModelProvider, params *entity.LLMParams) (model.BaseChatModel, error) {
	if instance.Connection.BaseConnInfo == nil {
		return nil, fmt.Errorf("model %s/%s has no base connection info", provider.ID, instance.ModelID)
	}

	conn := instance.Connection.BaseConnInfo

	cfg := &einoOpenAI.ChatModelConfig{
		Model:     conn.Model,
		APIKey:    conn.APIKey,
		MaxTokens: gptr.Of(4096),
		ResponseFormat: &einoOpenAI.ChatCompletionResponseFormat{
			Type: einoOpenAI.ChatCompletionResponseFormatTypeText,
		},
	}

	// Set BaseURL only for non-default OpenAI endpoints.
	if conn.BaseURL != "" {
		cfg.BaseURL = conn.BaseURL
	}

	if instance.Connection.Openai != nil {
		cfg.ByAzure = instance.Connection.Openai.ByAzure
		cfg.APIVersion = instance.Connection.Openai.APIVersion
	}

	applyParamsToOpenAIChatModelConfig(cfg, params)

	// Apply ThinkingLevel via OpenAI's ReasoningEffort config field.
	applyThinkingToOpenAIConfig(cfg, params, instance)

	return einoOpenAI.NewChatModel(ctx, cfg)
}

func applyParamsToOpenAIChatModelConfig(cfg *einoOpenAI.ChatModelConfig, params *entity.LLMParams) {
	if params == nil {
		return
	}

	if params.Temperature != nil {
		cfg.Temperature = params.Temperature
	}
	if params.MaxTokens != 0 {
		cfg.MaxTokens = gptr.Of(params.MaxTokens)
	}

	if params.FrequencyPenalty != 0 {
		cfg.FrequencyPenalty = gptr.Of(params.FrequencyPenalty)
	}

	if params.PresencePenalty != 0 {
		cfg.PresencePenalty = gptr.Of(params.PresencePenalty)
	}

	cfg.TopP = params.TopP

	if params.ResponseFormat == entity.ModelResponseFormatJSON {
		cfg.ResponseFormat = &einoOpenAI.ChatCompletionResponseFormat{
			Type: einoOpenAI.ChatCompletionResponseFormatTypeJSONObject,
		}
	}
}

// applyThinkingToOpenAIConfig applies ThinkingLevel to the OpenAI ChatModelConfig.
// Uses the OpenAI strategy to map ThinkingLevel → ReasoningEffort.
func applyThinkingToOpenAIConfig(cfg *einoOpenAI.ChatModelConfig, params *entity.LLMParams, instance *entity.ModelInstance) {
	level := resolveEffectiveThinkingLevel(params, instance)
	if !level.IsEnabled() {
		return
	}

	strategy := thinking.ForProvider("openai")
	clamped := thinking.ClampLevel(strategy, level)

	switch clamped {
	case entity.ThinkingLevelLow:
		cfg.ReasoningEffort = einoOpenAI.ReasoningEffortLevelLow
	case entity.ThinkingLevelMedium:
		cfg.ReasoningEffort = einoOpenAI.ReasoningEffortLevelMedium
	case entity.ThinkingLevelHigh, entity.ThinkingLevelXHigh:
		cfg.ReasoningEffort = einoOpenAI.ReasoningEffortLevelHigh
	}
}

// resolveEffectiveThinkingLevel determines the effective ThinkingLevel from params and model instance.
// Priority: params.ThinkingLevel > params.EnableThinking > model.Reasoning auto-detect.
func resolveEffectiveThinkingLevel(params *entity.LLMParams, instance *entity.ModelInstance) entity.ThinkingLevel {
	if params != nil && params.ThinkingLevel != "" {
		return params.ThinkingLevel
	}
	if params != nil && params.EnableThinking != nil {
		if *params.EnableThinking {
			return entity.ThinkingLevelLow
		}
		return entity.ThinkingLevelOff
	}
	return entity.ThinkingLevelOff
}
