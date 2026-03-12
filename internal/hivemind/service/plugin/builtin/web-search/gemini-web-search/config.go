// Package gemini_websearch implements the "web-search" built-in plugin.
//
// This plugin provides web search capabilities to agents via Gemini's
// Google Search grounding feature. It calls the Gemini generateContent
// API with the google_search tool enabled, returning AI-synthesized
// answers with citations from real-time Google Search results.
package gemini_websearch

// Config is the resolved configuration for the web-search plugin.
type Config struct {
	// Enabled controls whether the web-search plugin is active.
	Enabled bool `json:"enabled"`

	// Provider selects the search backend. Currently only "gemini" is supported.
	// Future: "brave", "perplexity", etc.
	Provider string `json:"provider"`

	// Gemini holds Gemini-specific configuration.
	Gemini GeminiConfig `json:"gemini"`

	// TimeoutSeconds is the HTTP request timeout in seconds.
	TimeoutSeconds int `json:"timeout_seconds"`
}

// GeminiConfig holds configuration for the Gemini search provider.
type GeminiConfig struct {
	// APIKey is the Gemini API key.
	// If empty, falls back to GEMINI_API_KEY environment variable.
	APIKey string `json:"api_key"`

	// Model is the Gemini model to use for grounded search.
	// Defaults to "gemini-2.5-flash".
	Model string `json:"model"`
}

const (
	defaultProvider       = "gemini"
	defaultGeminiModel    = "gemini-2.5-flash"
	defaultTimeoutSeconds = 30
)

// DefaultConfig returns sensible defaults for the web-search plugin.
func DefaultConfig() *Config {
	return &Config{
		Enabled:        false,
		Provider:       defaultProvider,
		TimeoutSeconds: defaultTimeoutSeconds,
		Gemini: GeminiConfig{
			Model: defaultGeminiModel,
		},
	}
}
