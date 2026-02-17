package entity

// MemoryConfig is the resolved configuration for the memory system.
// This corresponds to OpenClaw's ResolvedMemorySearchConfig.
type MemoryConfig struct {
	// Enabled controls whether the memory system is active.
	Enabled bool `json:"enabled"`

	// WorkspaceDir is the root directory for memory files.
	WorkspaceDir string `json:"workspace_dir"`

	// Sources defines which sources to index: "memory", "sessions".
	Sources []MemorySource `json:"sources"`

	// ExtraPaths are additional directories/files to index.
	ExtraPaths []string `json:"extra_paths"`

	// Embedding holds the embedding provider configuration.
	Embedding EmbeddingConfig `json:"embedding"`

	// Store holds the storage backend configuration.
	Store StoreConfig `json:"store"`

	// Chunking holds the chunking parameters.
	Chunking ChunkingConfig `json:"chunking"`

	// Sync holds the synchronization strategy.
	Sync SyncConfig `json:"sync"`

	// Query holds the search query parameters.
	Query QueryConfig `json:"query"`

	// Cache holds the embedding cache configuration.
	Cache CacheConfig `json:"cache"`
}

// EmbeddingConfig configures the embedding provider.
type EmbeddingConfig struct {
	// Provider is the embedding backend: "openai", "gemini", "local", "auto".
	Provider string `json:"provider"`

	// Model is the model name for embedding (e.g. "text-embedding-3-small").
	Model string `json:"model"`

	// Fallback is the fallback provider if primary fails.
	Fallback string `json:"fallback"`

	// Remote holds remote API configuration.
	Remote *RemoteEmbeddingConfig `json:"remote,omitempty"`
}

// RemoteEmbeddingConfig holds configuration for remote embedding APIs.
type RemoteEmbeddingConfig struct {
	// BaseURL is the API base URL.
	BaseURL string `json:"base_url"`

	// APIKey is the API key.
	APIKey string `json:"api_key"`

	// Headers are extra HTTP headers.
	Headers map[string]string `json:"headers,omitempty"`
}

// StoreConfig configures the storage backend.
type StoreConfig struct {
	// Driver is the storage driver (currently only "sqlite").
	Driver string `json:"driver"`

	// Path is the database file path.
	Path string `json:"path"`

	// Vector holds vector extension configuration.
	Vector VectorConfig `json:"vector"`
}

// VectorConfig configures the sqlite-vec extension.
type VectorConfig struct {
	// Enabled indicates whether to use the sqlite-vec extension.
	Enabled bool `json:"enabled"`

	// ExtensionPath is the path to the sqlite-vec shared library.
	ExtensionPath string `json:"extension_path,omitempty"`
}

// SyncConfig configures the synchronization strategy.
type SyncConfig struct {
	// OnSessionStart triggers sync when a session starts.
	OnSessionStart bool `json:"on_session_start"`

	// OnSearch triggers sync before each search.
	OnSearch bool `json:"on_search"`

	// Watch enables file watching for auto-sync.
	Watch bool `json:"watch"`

	// WatchDebounceMs is the debounce interval for file watch events.
	WatchDebounceMs int `json:"watch_debounce_ms"`

	// IntervalMinutes is the periodic sync interval.
	IntervalMinutes int `json:"interval_minutes"`
}

// CacheConfig configures the embedding cache.
type CacheConfig struct {
	// Enabled controls whether embedding caching is active.
	Enabled bool `json:"enabled"`

	// MaxEntries is the maximum number of cached embeddings.
	MaxEntries int `json:"max_entries,omitempty"`
}

// DefaultMemoryConfig returns a sensible default memory configuration.
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		Enabled:    true,
		Sources:    []MemorySource{MemorySourceMemory},
		ExtraPaths: nil,
		Embedding: EmbeddingConfig{
			Provider: "openai",
			Model:    "text-embedding-3-small",
			Fallback: "none",
		},
		Store: StoreConfig{
			Driver: "sqlite",
			Path:   ".echoryn/memory/index.db",
			Vector: VectorConfig{Enabled: false},
		},
		Chunking: DefaultChunkingConfig(),
		Sync: SyncConfig{
			OnSessionStart:  true,
			OnSearch:        true,
			Watch:           true,
			WatchDebounceMs: 1500,
			IntervalMinutes: 0,
		},
		Query: DefaultQueryConfig(),
		Cache: CacheConfig{
			Enabled:    true,
			MaxEntries: 10000,
		},
	}
}
