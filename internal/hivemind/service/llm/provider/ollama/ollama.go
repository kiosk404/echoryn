package ollama

import (
	"context"
	"fmt"

	"github.com/bytedance/gg/gptr"
	einoOllama "github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
	"github.com/kiosk404/echoryn/internal/pkg/options"
)

const Name = "ollama"

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
	if instance.Connection.BaseConnInfo == nil {
		return nil, fmt.Errorf("model %s/%s has no connection info", provider.ID, instance.ModelID)
	}

	conn := instance.Connection.BaseConnInfo
	conf := &einoOllama.ChatModelConfig{
		BaseURL: "http://127.0.0.1:11434/v1",
		Model:   conn.Model,
		Options: &einoOllama.Options{},
	}
	if conn.BaseURL != "" {
		conf.BaseURL = conn.BaseURL
	}

	switch conn.ThinkingType {
	case entity.ThinkingType_Enable:
		conf.Thinking = &einoOllama.ThinkValue{
			Value: gptr.Of(true),
		}
	case entity.ThinkingType_Disable:
		conf.Thinking = &einoOllama.ThinkValue{
			Value: gptr.Of(false),
		}
	}

	applyParamsToOllamaConfig(conf, params)

	return einoOllama.NewChatModel(ctx, conf)
}

// applyParamsToOllamaConfig applies runtime LLM params to the Ollama config.
func applyParamsToOllamaConfig(conf *einoOllama.ChatModelConfig, params *entity.LLMParams) {
	if params == nil {
		return
	}

	if params.Temperature != nil {
		conf.Options.Temperature = *params.Temperature
	}
	if params.TopP != nil {
		conf.Options.TopP = *params.TopP
	}
	if params.TopK != nil {
		conf.Options.TopK = int(*params.TopK)
	}
	if params.FrequencyPenalty != 0 {
		conf.Options.FrequencyPenalty = params.FrequencyPenalty
	}
	if params.PresencePenalty != 0 {
		conf.Options.PresencePenalty = params.PresencePenalty
	}
	if params.EnableThinking != nil {
		conf.Thinking = &einoOllama.ThinkValue{
			Value: params.EnableThinking,
		}
	}
}

func (p *Plugin) DefaultConfig() *options.ProviderConfig {
	return &options.ProviderConfig{
		BaseURL: "http://127.0.0.1:11434/v1",
		APIKey:  "${OLLAMA_API_KEY}",
		API:     "openai-completions",
		Models:  []options.ModelDefinition{},
	}
}
