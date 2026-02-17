package deepseek

import (
	"context"
	"fmt"

	einoDeepseek "github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
	"github.com/kiosk404/echoryn/internal/pkg/options"
)

const Name = "deepseek"

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

	conf := &einoDeepseek.ChatModelConfig{
		APIKey:      conn.APIKey,
		Model:       conn.Model,
		Temperature: temperature,
	}

	if conn.BaseURL != "" {
		conf.BaseURL = conn.BaseURL
	}

	applyParamsToDeepseekConfig(conf, params)

	return einoDeepseek.NewChatModel(ctx, conf)
}

// applyParamsToDeepseekConfig maps LLMParams to Deepseek ChatModelConfig.
func applyParamsToDeepseekConfig(conf *einoDeepseek.ChatModelConfig, params *entity.LLMParams) {
	if params == nil {
		return
	}

	if params.Temperature != nil {
		conf.Temperature = *params.Temperature
	}

	if params.MaxTokens != 0 {
		conf.MaxTokens = params.MaxTokens
	}

	if params.FrequencyPenalty != 0 {
		conf.FrequencyPenalty = params.FrequencyPenalty
	}

	if params.PresencePenalty != 0 {
		conf.PresencePenalty = params.PresencePenalty
	}

	if params.ResponseFormat == entity.ModelResponseFormatJSON {
		conf.ResponseFormatType = einoDeepseek.ResponseFormatTypeJSONObject
	} else {
		conf.ResponseFormatType = einoDeepseek.ResponseFormatTypeText
	}
}

func (p *Plugin) DefaultConfig() *options.ProviderConfig {
	return &options.ProviderConfig{
		BaseURL: "https://api.deepseek.com/v1",
		APIKey:  "${DEEPSEEK_API_KEY}",
		API:     "openai-completions",
		Models: []options.ModelDefinition{
			{ID: "deepseek-chat", Name: "Deepseek V3", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192, Cost: options.ModelCost{Input: 0.27, Output: 1.1, CacheRead: 0.07}},
			{ID: "deepseek-reasoner", Name: "Deepseek R1", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192, Cost: options.ModelCost{Input: 0.55, Output: 2.19, CacheRead: 0.14}},
		},
	}
}
