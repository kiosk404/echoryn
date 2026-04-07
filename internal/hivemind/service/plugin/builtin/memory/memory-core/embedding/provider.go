package embedding

import (
	"context"
)

// Provider is the interface that all embedding backends must implement.
type Provider interface {
	// ID return the provider identity (e.g. "openai", "gemini", "local")
	ID() string
	// Model returns the model name (e.g. "text-embedding-ada-002", "gemini-embedding")
	Model() string
	// EmbedQuery embeds a single query text into a vector.
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	// EmbedBatch embeds multiple query texts into vectors.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// ProviderResult holds the result of a provider selection.
type ProviderResult struct {
	// Provider is the created embedding provider.
	Provider Provider
	// RequestedBackend is the optionally requested backend. ("openai", "gemini", "local")
	RequestedBackend string
	// FallbackFrom is the backend that was used as a fallback.
	FallbackFrom string
	// FallbackReason is the reason why a fallback was used.
	FallbackReason string
}

// ProviderKey returns a stable key identifier for the provider.
// used for embedding cache lookups.
func ProviderKey(p Provider) string {
	return p.ID() + ":" + p.Model()
}
