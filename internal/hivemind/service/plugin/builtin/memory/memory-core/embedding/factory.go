package embedding

import (
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory/memory-core/entity"
)

// NewProvider creates an embedding provider based on the config.
// Supports "openai", "gemini", "ollama" (local), with fallback logic.
// Auto mode: ollama (if reachable) → openai → gemini.
// Matches OpenClaw's createEmbeddingProvider.
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
		case "gemini":
			apiKey := ""
			baseURL := ""
			if cfg.Remote != nil {
				apiKey = cfg.Remote.APIKey
				baseURL = cfg.Remote.BaseURL
			}
			if apiKey == "" {
				return nil, fmt.Errorf("no API key found for provider gemini")
			}
			return NewGeminiProvider(GeminiOptions{
				APIKey:  apiKey,
				BaseURL: baseURL,
				Model:   cfg.Model,
			}), nil
		case "ollama":
			baseURL := ""
			if cfg.Remote != nil && cfg.Remote.BaseURL != "" {
				baseURL = cfg.Remote.BaseURL
			}
			if !IsOllamaAvailable(baseURL) {
				return nil, fmt.Errorf("ollama server not reachable at %s", OllamaBaseURL(baseURL))
			}
			return NewOllamaProvider(OllamaOptions{
				BaseURL: baseURL,
				Model:   cfg.Model,
			}), nil
		case "auto":
			// Auto mode: try ollama (local, free) → openai → gemini.
			p, errOllama := createByID("ollama")
			if errOllama == nil {
				return p, nil
			}
			p, errOpenAI := createByID("openai")
			if errOpenAI == nil {
				return p, nil
			}
			p, errGemini := createByID("gemini")
			if errGemini == nil {
				return p, nil
			}
			return nil, fmt.Errorf("no embeddings provider available (tried ollama: %v; openai: %v; gemini: %v)", errOllama, errOpenAI, errGemini)
		default:
			return nil, fmt.Errorf("unsupported embedding provider: %q", id)
		}
	}

	provider, err := createByID(requested)
	if err != nil {
		// Try fallback.
		if cfg.Fallback != "" && cfg.Fallback != "none" && cfg.Fallback != requested {
			fallbackProvider, fallbackErr := createByID(cfg.Fallback)
			if fallbackErr != nil {
				return nil, fmt.Errorf("%w; fallback to %s failed: %w", err, cfg.Fallback, fallbackErr)
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
