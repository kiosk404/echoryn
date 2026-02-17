package helper

import (
	"fmt"
	"os"
	"strings"

	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/pkg/options"
)

type BasePlugin struct {
	PluginName string
}

func (b *BasePlugin) Name() string {
	return b.PluginName
}

// DefaultConfig returns the default configuration for the provider.
func (b *BasePlugin) DefaultConfig() *options.ProviderConfig {
	return &options.ProviderConfig{}
}

// BuildProvider constructs a ModelProvider from config with sensible defaults.
func (b *BasePlugin) BuildProvider(cfg *options.ProviderConfig) (*entity.ModelProvider, error) {
	apiKey := ResolveEnvValue(cfg.APIKey)
	api, err := entity.ModelAPIFromString(cfg.API)
	if err != nil {
		return nil, fmt.Errorf("invalid API for provider %q: %w", b.PluginName, err)
	}

	authHeader := true
	if cfg.AuthHeader != nil {
		authHeader = *cfg.AuthHeader
	}

	return &entity.ModelProvider{
		ID:         b.PluginName,
		ModelClass: entity.ModelClassFromString(b.PluginName),
		BaseURL:    cfg.BaseURL,
		APIKey:     apiKey,
		API:        api,
		AuthHeader: authHeader,
		Headers:    cfg.Headers,
		Enabled:    true,
		Name: &entity.I18nText{
			EnUs: b.PluginName,
			ZhCn: b.PluginName,
		},
	}, nil
}

// BuildModels constructs ModelInstance entities from the provider config's model definitions.
func (b *BasePlugin) BuildModels(p *entity.ModelProvider, cfg *options.ProviderConfig) ([]*entity.ModelInstance, error) {
	var models []*entity.ModelInstance
	apiKey := ResolveEnvValue(cfg.APIKey)

	for _, modelDef := range cfg.Models {
		modelAPI := p.API
		if modelDef.API != "" {
			if parsed, err := entity.ModelAPIFromString(modelDef.API); err == nil {
				modelAPI = parsed
			}
		}

		inputTypes := modelDef.Input
		if len(inputTypes) == 0 {
			inputTypes = []string{"text"}
		}

		displayName := modelDef.Name
		if displayName == "" {
			displayName = modelDef.ID
		}

		instance := &entity.ModelInstance{
			ModelID:    modelDef.ID,
			ProviderID: p.ID,
			Type:       entity.ModelType_LLM,
			DisplayInfo: entity.DisplayInfo{
				Name:      displayName,
				MaxTokens: int64(modelDef.MaxTokens),
			},
			Connection: entity.Connection{
				BaseConnInfo: &entity.BaseConnectionInfo{
					BaseURL: cfg.BaseURL,
					APIKey:  apiKey,
					Model:   modelDef.ID,
				},
			},
			Capability: BuildCapabilityFromInputs(inputTypes, modelDef.Reasoning),
			Cost: entity.ModelCostInfo{
				Input:      modelDef.Cost.Input,
				Output:     modelDef.Cost.Output,
				CacheRead:  modelDef.Cost.CacheRead,
				CacheWrite: modelDef.Cost.CacheWrite,
			},
			ContextWindow: modelDef.ContextWindow,
			MaxTokens:     modelDef.MaxTokens,
			Reasoning:     modelDef.Reasoning,
			InputTypes:    inputTypes,
			Status:        entity.ModelStatus_Ready,
		}

		ApplyProviderConnection(instance, p, modelAPI)
		models = append(models, instance)
	}

	return models, nil
}

// BuildCapabilityFromInputs derives ModelAbility from input types and reasoning flag.
func BuildCapabilityFromInputs(inputs []string, reasoning bool) entity.ModelAbility {
	ability := entity.ModelAbility{
		FunctionCall: true,
		CotDisplay:   reasoning,
	}
	for _, input := range inputs {
		switch input {
		case "image":
			ability.ImageUnderstanding = true
			ability.SupportMultiModal = true
		case "audio":
			ability.AudioUnderstanding = true
			ability.SupportMultiModal = true
		case "video":
			ability.VideoUnderstanding = true
			ability.SupportMultiModal = true
		}
	}
	return ability
}

// ApplyProviderConnection sets provider-specific connection fields on a model instance.
func ApplyProviderConnection(instance *entity.ModelInstance, p *entity.ModelProvider, api entity.ModelAPI) {
	switch p.ModelClass {
	case entity.ModelClass_GPT:
		instance.Connection.Openai = &entity.OpenAIConnInfo{}
	case entity.ModelClass_DeepSeek:
		instance.Connection.Deepseek = &entity.DeepseekConnInfo{}
	case entity.ModelClass_Gemini:
		instance.Connection.Gemini = &entity.GeminiConnInfo{}
	case entity.ModelClass_QWen:
		instance.Connection.Qwen = &entity.QwenConnInfo{}
	case entity.ModelClass_Ollama:
		instance.Connection.Ollama = &entity.OllamaConnInfo{}
	case entity.ModelClass_Claude:
		instance.Connection.Claude = &entity.ClaudeConnInfo{}
	}

	if instance.Reasoning {
		instance.Connection.BaseConnInfo.ThinkingType = entity.ThinkingType_Enable
	}

	_ = api
}

// ResolveEnvValue resolves "${ENV_VAR}" references in a string.
func ResolveEnvValue(s string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		envKey := s[2 : len(s)-1]
		return os.Getenv(envKey)
	}
	return s
}
