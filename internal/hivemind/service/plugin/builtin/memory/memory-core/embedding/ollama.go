package embedding

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

const (
	// defaultOllamaBaseURL is the default Ollama API base URL (OpenAI-compatible endpoint).
	defaultOllamaBaseURL = "http://localhost:11434/v1"

	// defaultOllamaModel is the default embedding model for Ollama.
	// nomic-embed-text is lightweight (274MB) and widely available.
	defaultOllamaModel = "nomic-embed-text"

	// ollamaHealthTimeout is the timeout for checking if Ollama is reachable.
	ollamaHealthTimeout = 2 * time.Second
)

// ollamaProvider implements Provider by delegating to an openAIProvider
// pointed at the local Ollama server's OpenAI-compatible endpoint.
// Ollama exposes /v1/embeddings with the same request/response format as OpenAI.
type ollamaProvider struct {
	inner   *openAIProvider
	baseURL string
}

// OllamaOptions configures the Ollama embedding provider.
type OllamaOptions struct {
	// BaseURL is the Ollama server URL. Defaults to "http://localhost:11434/v1".
	BaseURL string

	// Model is the embedding model name. Defaults to "nomic-embed-text".
	Model string
}

// NewOllamaProvider creates a local Ollama embedding provider.
// It reuses the OpenAI HTTP logic since Ollama's /v1/embeddings API is fully compatible.
func NewOllamaProvider(opts OllamaOptions) Provider {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	model := opts.Model
	if model == "" {
		model = defaultOllamaModel
	}

	inner := NewOpenAIProvider(OpenAIOptions{
		APIKey:  "ollama", // Ollama doesn't require a real API key.
		BaseURL: baseURL,
		Model:   model,
	}).(*openAIProvider)

	// Use a shorter timeout for local requests.
	inner.client = &http.Client{
		Timeout: 30 * time.Second,
	}

	return &ollamaProvider{
		inner:   inner,
		baseURL: baseURL,
	}
}

func (p *ollamaProvider) ID() string    { return "ollama" }
func (p *ollamaProvider) Model() string { return p.inner.Model() }

func (p *ollamaProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return p.inner.EmbedQuery(ctx, text)
}

func (p *ollamaProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return p.inner.EmbedBatch(ctx, texts)
}

// IsOllamaAvailable checks if an Ollama server is reachable at the given base URL
// by attempting a TCP connection to the host. This is used by the "auto" provider
// selection to detect local Ollama before falling back to remote APIs.
func IsOllamaAvailable(baseURL string) bool {
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}

	// Extract host:port from the base URL.
	// Default Ollama listens on localhost:11434.
	host := "localhost:11434"
	if baseURL != defaultOllamaBaseURL {
		// Parse host from custom URL.
		// Strip scheme.
		u := baseURL
		for _, prefix := range []string{"http://", "https://"} {
			if len(u) > len(prefix) && u[:len(prefix)] == prefix {
				u = u[len(prefix):]
				break
			}
		}
		// Strip path.
		for i, c := range u {
			if c == '/' {
				u = u[:i]
				break
			}
		}
		if u != "" {
			host = u
		}
		// Add default port if missing.
		if _, _, err := net.SplitHostPort(host); err != nil {
			host = host + ":11434"
		}
	}

	conn, err := net.DialTimeout("tcp", host, ollamaHealthTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// OllamaBaseURL returns the resolved base URL for display/logging.
func OllamaBaseURL(baseURL string) string {
	if baseURL == "" {
		return defaultOllamaBaseURL
	}
	return fmt.Sprintf("%s", baseURL)
}
