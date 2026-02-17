package helper

import (
	"context"
	"fmt"

	"github.com/bytedance/gg/gptr"
	einoOpenAI "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
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
