package embedding

import (
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/entity"
)

// NewProvider creates a new embedding provider based on the configuration.
func NewProvider(cfg entity.EmbeddingConfig) (*ProviderResult, error) {
	requested := cfg.Provider

	var createByID func(id string) (Provider, error)
	createByID = func(id string) (Provider, error) {
		switch id {
		case "openai":
			apiKey := ""
			baseURL := ""
			if cfg.Remote != nil {
				apiKey = cfg.Remote.APIKey
				baseURL = cfg.Remote.BaseURL
			}
			if apiKey == "" {
				return nil, fmt.Errorf("no API key found for provider openai")
			}
			return NewOpenAIProvider(OpenAIOptions{
				APIKey:  apiKey,
				BaseURL: baseURL,
				Model:   cfg.Model,
			}), nil
		case "auto":
			p, err := createByID("openai")
			if err == nil {
				return p, nil
			}
			return nil, fmt.Errorf("no embedding provider available (tried openai): %w", err)
		default:
			return nil, fmt.Errorf("unsupported embedding provider: %s", id)
		}
	}

	provider, err := createByID(requested)
	if err != nil {
		// Try fallback
		if cfg.Fallback != "" && cfg.Fallback != "none" && cfg.Fallback != requested {
			fallbackProvider, fallbackErr := createByID(cfg.Fallback)
			if fallbackErr != nil {
				return nil, fmt.Errorf("no fallback embedding provider available (tried %s): %w", cfg.Fallback, fallbackErr)
			}
			return &ProviderResult{
				Provider:         fallbackProvider,
				RequestedBackend: requested,
				FallbackFrom:     requested,
				FallbackReason:   err.Error(),
			}, nil
		}
		return nil, err
	}

	return &ProviderResult{
		Provider:         provider,
		RequestedBackend: requested,
	}, nil
}
