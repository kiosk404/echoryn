package gemini

import (
	"context"
	"fmt"

	einoGemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/helper"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm/provider/spi"
	"github.com/kiosk404/echoryn/internal/pkg/options"
	"google.golang.org/genai"
)

const Name = "gemini"

// Compile-time check: Plugin implements ChatModelPlugin.
var _ spi.ChatModelPlugin = (*Plugin)(nil)

type Plugin struct {
	helper.BasePlugin
}

func New() spi.ProviderPlugin {
	return &Plugin{
		BasePlugin: helper.BasePlugin{PluginName: Name},
	}
}

// BuildChatModel overrides BasePlugin's default OpenAI-compatible implementation
// because Gemini uses Google's generative AI API (google-generative-ai).
// Follows the airi-go geminiModelBuilder pattern with full LLMParams support.
func (p *Plugin) BuildChatModel(ctx context.Context, instance *entity.ModelInstance, provider *entity.ModelProvider, params *entity.LLMParams) (model.BaseChatModel, error) {
	if instance.Connection.BaseConnInfo == nil {
		return nil, fmt.Errorf("model %s/%s has no base connection info", provider.ID, instance.ModelID)
	}

	conn := instance.Connection.BaseConnInfo

	// Build genai.ClientConfig.
	clientCfg := &genai.ClientConfig{
		APIKey:  conn.APIKey,
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: "https://generativelanguage.googleapis.com/",
		},
	}

	if conn.BaseURL != "" {
		clientCfg.HTTPOptions.BaseURL = conn.BaseURL
	}

	// Apply Gemini-specific connection fields (Vertex AI support).
	if instance.Connection.Gemini != nil {
		g := instance.Connection.Gemini
		if g.Backend != 0 {
			clientCfg.Backend = genai.Backend(g.Backend)
		}
		clientCfg.Project = g.Project
		clientCfg.Location = g.Location
	}

	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("create genai client for %s/%s: %w", provider.ID, instance.ModelID, err)
	}

	cfg := &einoGemini.Config{
		Client: client,
		Model:  conn.Model,
	}

	// Apply ThinkingType from connection config.
	switch conn.ThinkingType {
	case entity.ThinkingType_Enable:
		cfg.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: true,
		}
	case entity.ThinkingType_Disable:
		cfg.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: false,
		}
	}

	// Apply runtime LLM params.
	applyParamsToGeminiConfig(cfg, params)

	return einoGemini.NewChatModel(ctx, cfg)
}

// applyParamsToGeminiConfig maps LLMParams to Gemini Config.
// Follows the airi-go geminiModelBuilder.applyParamsToGeminiConfig pattern.
func applyParamsToGeminiConfig(conf *einoGemini.Config, params *entity.LLMParams) {
	if params == nil {
		return
	}

	conf.TopK = params.TopK
	conf.TopP = params.TopP

	if params.Temperature != nil {
		t := *params.Temperature
		conf.Temperature = &t
	}

	if params.MaxTokens != 0 {
		mt := params.MaxTokens
		conf.MaxTokens = &mt
	}

	if params.EnableThinking != nil {
		conf.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: *params.EnableThinking,
		}
	}
}

func (p *Plugin) DefaultConfig() *options.ProviderConfig {
	return &options.ProviderConfig{
		BaseURL: "https://generativelanguage.googleapis.com/v1beta",
		APIKey:  "${GOOGLE_API_KEY}",
		API:     "google-generative-ai",
		Models: []options.ModelDefinition{
			{ID: "gemini-2.5-pro-preview-06-05", Name: "Gemini 2.5 Pro", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536, Cost: options.ModelCost{Input: 1.25, Output: 10, CacheRead: 0.31}},
			{ID: "gemini-2.5-flash-preview-05-20", Name: "Gemini 2.5 Flash", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536, Cost: options.ModelCost{Input: 0.15, Output: 0.6, CacheRead: 0.0375}},
			{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 8192, Cost: options.ModelCost{Input: 0.1, Output: 0.4, CacheRead: 0.025}},
		},
	}
}
