package thinking

import (
	"sync"
)

// registry holds the mapping from provider name to ThinkingStrategy.
// Initialized once via init() with builtin strategies.
var (
	registryMu sync.RWMutex
	strategies = make(map[string]Strategy)
)

func init() {
	// Register builtin strategies for all known providers.
	Register("openai", &OpenAIStrategy{})
	Register("anthropic", &ClaudeStrategy{})
	Register("gemini", &GeminiStrategy{})
	Register("qwen", &QwenStrategy{})

	// Noop strategies for providers with config-level-only or model-intrinsic thinking.
	Register("deepseek", &NoopStrategy{ProviderName: "deepseek"})
	Register("ollama", &NoopStrategy{ProviderName: "ollama"})
	Register("kimi", &NoopStrategy{ProviderName: "kimi"})
	Register("glm", &NoopStrategy{ProviderName: "glm"})
}

// Register adds a ThinkingStrategy for the given provider name.
// Safe for concurrent use; later registrations overwrite earlier ones.
func Register(providerName string, s Strategy) {
	registryMu.Lock()
	defer registryMu.Unlock()
	strategies[providerName] = s
}

// ForProvider returns the ThinkingStrategy for the given provider name.
// Returns a NoopStrategy if no specific strategy is registered.
func ForProvider(providerName string) Strategy {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if s, ok := strategies[providerName]; ok {
		return s
	}
	return &NoopStrategy{ProviderName: providerName}
}
