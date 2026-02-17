package qwen

import (
	"context"
	"fmt"

	"github.com/bytedance/gg/gptr"
	einoOpenAI "github.com/cloudwego/eino-ext/components/model/openai"
	einoQwen "github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
	"github.com/kiosk404/echoryn/internal/pkg/options"
)

const Name = "qwen"

var _ spi.ChatModelPlugin = (*Plugin)(nil)

type Plugin struct {
	helper.BasePlugin
}

func New() spi.ProviderPlugin {
	return &Plugin{
		BasePlugin: helper.BasePlugin{PluginName: Name},
	}
}

// BuildChatModel overrides BasePlugin to use the dedicated Qwen SDK
// with full LLMParams support, following the airi-go qwenModelBuilder pattern.
func (p *Plugin) BuildChatModel(ctx context.Context, instance *entity.ModelInstance, provider *entity.ModelProvider, params *entity.LLMParams) (model.BaseChatModel, error) {
	if instance.Connection.BaseConnInfo == nil {
		return nil, fmt.Errorf("model %s/%s has no base connection info", provider.ID, instance.ModelID)
	}

	temperature := float32(0.7)
	conn := instance.Connection.BaseConnInfo

	conf := &einoQwen.ChatModelConfig{
		APIKey:      conn.APIKey,
		Model:       conn.Model,
		Temperature: gptr.Of(temperature),
		ResponseFormat: &einoOpenAI.ChatCompletionResponseFormat{
			Type: "text",
		},
	}

	if conn.BaseURL != "" {
		conf.BaseURL = conn.BaseURL
	}

	// Apply ThinkingType from connection config.
	switch conn.ThinkingType {
	case entity.ThinkingType_Enable:
		conf.EnableThinking = gptr.Of(true)
	case entity.ThinkingType_Disable:
		conf.EnableThinking = gptr.Of(false)
	}

	applyParamsToQwenConfig(conf, params)

	return einoQwen.NewChatModel(ctx, conf)
}

// applyParamsToQwenConfig maps LLMParams to Qwen ChatModelConfig.
func applyParamsToQwenConfig(conf *einoQwen.ChatModelConfig, params *entity.LLMParams) {
	if params == nil {
		return
	}

	conf.TopP = params.TopP

	if params.Temperature != nil {
		conf.Temperature = gptr.Of(*params.Temperature)
	}

	if params.MaxTokens != 0 {
		conf.MaxTokens = gptr.Of(params.MaxTokens)
	}

	if params.FrequencyPenalty != 0 {
		conf.FrequencyPenalty = gptr.Of(params.FrequencyPenalty)
	}

	if params.PresencePenalty != 0 {
		conf.PresencePenalty = gptr.Of(params.PresencePenalty)
	}

	if params.EnableThinking != nil {
		conf.EnableThinking = params.EnableThinking
	}
}

func (p *Plugin) DefaultConfig() *options.ProviderConfig {
	return &options.ProviderConfig{
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		APIKey:  "${DASHSCOPE_API_KEY}",
		API:     "openai-completions",
		Models: []options.ModelDefinition{
			{ID: "qwen-plus", Name: "Qwen Plus", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192, Cost: options.ModelCost{Input: 0.8, Output: 2}},
			{ID: "qwen-turbo", Name: "Qwen Turbo", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192, Cost: options.ModelCost{Input: 0.3, Output: 0.6}},
			{ID: "qwen-max", Name: "Qwen Max", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192, Cost: options.ModelCost{Input: 2.4, Output: 9.6}},
			{ID: "qwq-plus", Name: "QWQ Plus (Reasoning)", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192, Cost: options.ModelCost{Input: 0.8, Output: 2}},
		},
	}
}
