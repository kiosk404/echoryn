package entity

// MemorySource indicates where a memory chunk originated from.
type MemorySource string

const (
	MemorySourceMemory   MemorySource = "memory"
	MemorySourceSessions MemorySource = "sessions"
)

// MemoryFileEntry represents a scanned memory file with its metadata and content hash.
type MemoryFileEntry struct {
	// Path is the relative path from workspace root.
	Path string

	// AbsPath is the absolute filesystem path.
	AbsPath string

	// MtimeMs is the file modification time in milliseconds since epoch.
	MtimeMs int64

	// Size is the file size in bytes.
	Size int64

	// Hash is the SHA-256 hash of the file content.
	Hash string
}

// MemoryChunk represents a chunk of a memory file, ready for embedding.
type MemoryChunk struct {
	// StartLine is the 1-based line number where this chunk begins.
	StartLine int

	// EndLine is the 1-based line number where this chunk ends.
	EndLine int

	// Text is the chunk content.
	Text string

	// Hash is the SHA-256 hash of the chunk text.
	Hash string
}

// MemorySearchResult is the final result returned by a memory search.
type MemorySearchResult struct {
	// Path is the file relative path.
	Path string `json:"path"`

	// StartLine is the match start line (1-based).
	StartLine int `json:"start_line"`

	// EndLine is the match end line (1-based).
	EndLine int `json:"end_line"`

	// Score is the relevance score (0-1).
	Score float64 `json:"score"`

	// Snippet is the truncated text snippet (<= SnippetMaxChars).
	Snippet string `json:"snippet"`

	// Source indicates the origin (memory or sessions).
	Source MemorySource `json:"source"`
}

// SessionFileEntry represents a parsed session transcript file.
type SessionFileEntry struct {
	// Path is the relative path (sessions/<filename>).
	Path string

	// AbsPath is the absolute filesystem path.
	AbsPath string

	// MtimeMs is the file modification time in milliseconds since epoch.
	MtimeMs int64

	// Size is the file size in bytes.
	Size int64

	// Hash is the SHA-256 hash of the extracted text content.
	Hash string

	// Content is the extracted text from session messages.
	Content string
}

// ChunkingConfig holds the configuration for Markdown chunking.
type ChunkingConfig struct {
	// Tokens is the maximum number of tokens per chunk (approx chars/4).
	Tokens int `json:"tokens"`

	// Overlap is the overlap tokens between consecutive chunks.
	Overlap int `json:"overlap"`
}

// DefaultChunkingConfig returns the default chunking parameters matching OpenClaw defaults.
func DefaultChunkingConfig() ChunkingConfig {
	return ChunkingConfig{
		Tokens:  400,
		Overlap: 80,
	}
}

// QueryConfig holds the query parameters for hybrid search.
type QueryConfig struct {
	// MaxResults is the maximum number of results to return.
	MaxResults int `json:"max_results"`

	// MinScore is the minimum relevance score threshold.
	MinScore float64 `json:"min_score"`

	// Hybrid contains hybrid search weights.
	Hybrid HybridConfig `json:"hybrid"`

	// MMR configures Maximal Marginal Relevance re-ranking for diversity.
	MMR MMRConfig `json:"mmr"`

	// TemporalDecay configures time-based score decay for recency-aware scouring
	TemporalDecay TemporalDecayConfig `json:"temporal_decay"`
}

// HybridConfig holds the weights for hybrid search merge.
type HybridConfig struct {
	// Enabled indicates whether hybrid search is active.
	Enabled bool `json:"enabled"`

	// VectorWeight is the weight for vector search results (default 0.7).
	VectorWeight float64 `json:"vector_weight"`

	// TextWeight is the weight for text/keyword search results (default 0.3).
	TextWeight float64 `json:"text_weight"`

	// CandidateMultiplier controls how many extra candidates to fetch.
	CandidateMultiplier float64 `json:"candidate_multiplier"`
}

// MMRConfig configures Maximal Marginal Relevance re-ranking.
// MMR balances relevance with diversity by penalizing results that are
// too similar to already-selected results.
type MMRConfig struct {
	// Enabled controls whether MMR re-ranking is applied. Default: false (opt-in).
	Enabled bool `json:"enabled"`

	// Lambda controls the trade-off between relevance and diversity
	// o = max diversity, 1 = max relevance, Default: 0.7.
	Lambda float64 `json:"lambda"`
}

// TemporalDecayConfig configures time-based score decay.
// Never documents score higher; older documents are penalized with exponential decay.
type TemporalDecayConfig struct {
	// Enabled controls whether temporal decay is applied. Default: false (opt-in)
	Enabled bool `json:"enabled"`

	// HalfLifeDays is the number of days after which a document's score is halved.
	// Default: 30.
	HalfLifeDays float64 `json:"half_life_days"`
}

// DefaultQueryConfig returns the default query config matching OpenClaw defaults.
func DefaultQueryConfig() QueryConfig {
	return QueryConfig{
		MaxResults: 6,
		MinScore:   0.35,
		Hybrid: HybridConfig{
			Enabled:             true,
			VectorWeight:        0.7,
			TextWeight:          0.3,
			CandidateMultiplier: 3,
		},
		MMR: MMRConfig{
			Enabled: false,
			Lambda:  0.7,
		},
		TemporalDecay: TemporalDecayConfig{
			Enabled:      false,
			HalfLifeDays: 30,
		},
	}
}
