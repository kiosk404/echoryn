package glm

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
	"github.com/kiosk404/echoryn/internal/pkg/options"
)

const Name = "glm"

var _ spi.ChatModelPlugin = (*Plugin)(nil)

type Plugin struct {
	helper.BasePlugin
}

func New() spi.ProviderPlugin {
	return &Plugin{
		BasePlugin: helper.BasePlugin{PluginName: Name},
	}
}

func (p *Plugin) BuildChatModel(ctx context.Context, instance *entity.ModelInstance, provider *entity.ModelProvider, params *entity.LLMParams) (model.BaseChatModel, error) {
	return helper.NewOpenAICompatibleChatModel(ctx, instance, provider, params)
}

func (p *Plugin) DefaultConfig() *options.ProviderConfig {
	return &options.ProviderConfig{
		BaseURL: "https://open.bigmodel.cn/api/paas/v4",
		APIKey:  "{ZHIPU_API_KEY}",
		API:     "openai-completions",
		Models: []options.ModelDefinition{
			{ID: "glm-4.6", Name: "GLM-4.6", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192, Cost: options.ModelCost{Input: 0.27, Output: 1.1, CacheRead: 0.07}},
			{ID: "glm-5", Name: "GLM-5", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192, Cost: options.ModelCost{Input: 0.27, Output: 1.1, CacheRead: 0.07}},
		},
	}
}
