package anthropic

import (
	"context"
	"fmt"

	einoClaude "github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
	"github.com/kiosk404/echoryn/internal/pkg/options"
)

const Name = "anthropic"

var _ spi.ChatModelPlugin = (*Plugin)(nil)

type Plugin struct {
	helper.BasePlugin
}

func New() spi.ProviderPlugin {
	return &Plugin{
		BasePlugin: helper.BasePlugin{PluginName: Name},
	}
}

func (p Plugin) BuildChatModel(ctx context.Context, instance *entity.ModelInstance, provider *entity.ModelProvider, params *entity.LLMParams) (model.BaseChatModel, error) {
	if instance.Connection.BaseConnInfo == nil {
		return nil, fmt.Errorf("model %s/%s has no connection info", provider.ID, instance.ModelID)
	}

	conn := instance.Connection.BaseConnInfo
	cfg := &einoClaude.Config{
		APIKey:    conn.APIKey,
		Model:     conn.Model,
		MaxTokens: instance.MaxTokens,
	}

	if conn.BaseURL != "" {
		cfg.BaseURL = &conn.BaseURL
	}

	// apply runtime LLM params
	applyParamsToClaudeConfig(cfg, params)

	return einoClaude.NewChatModel(ctx, cfg)
}

func applyParamsToClaudeConfig(conf *einoClaude.Config, params *entity.LLMParams) {
	if params == nil {
		return
	}

	if params.Temperature != nil {
		conf.Temperature = params.Temperature
	}
	if params.MaxTokens != 0 {
		conf.MaxTokens = params.MaxTokens
	}
	if params.TopP != nil {
		conf.TopP = params.TopP
	}
}

func (p *Plugin) DefaultConfig() *options.ProviderConfig {
	return &options.ProviderConfig{
		BaseURL: "https://api.anthropic.com/v1",
		APIKey:  "${ANTHROPIC_API_KEY}",
		API:     "anthropic-messages",
		Models: []options.ModelDefinition{
			{
				ID:            "claude-opus-4-6",
				Name:          "Claude Opus 4.6",
				Reasoning:     true,
				Input:         []string{"text"},
				ContextWindow: 200000, // 200K is the standard window; 1M window may be beta
				MaxTokens:     128000, // up to 128K output supported
				Cost: options.ModelCost{
					Input:  0.005, // API list price per million tokens
					Output: 0.025,
				},
			},
			{
				ID:            "claude-sonnet-4-5",
				Name:          "Claude Sonnet 4.5",
				Reasoning:     true,
				Input:         []string{"text"},
				ContextWindow: 200000,
				MaxTokens:     64000,
				Cost: options.ModelCost{
					Input:  0.003,
					Output: 0.015,
				},
			},
			{
				ID:            "claude-haiku-4-5",
				Name:          "Claude Haiku 4.5",
				Reasoning:     false,
				Input:         []string{"text"},
				ContextWindow: 200000,
				MaxTokens:     64000,
				Cost: options.ModelCost{
					Input:  0.001,
					Output: 0.005,
				},
			},
		},
	}
}
