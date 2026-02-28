package hybrid

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/entity"
)

// VectorResult holds a single vector search result.
type VectorResult struct {
	ID          string
	Path        string
	StartLine   int
	EndLine     int
	Source      entity.MemorySource
	Snippet     string
	VectorScore float64
}

// KeywordResult holds a single keyword search result.
type KeywordResult struct {
	ID        string
	Path      string
	StartLine int
	EndLine   int
	Source    entity.MemorySource
	Snippet   string
	TextScore float64
}

// HybridResult is an intermediate result with the merged score, used for post-processing
// (temporal decay, MMR) before converting to the final MemorySearchResult.
type HybridResult struct {
	Path      string
	StartLine int
	EndLine   int
	Score     float64
	Snippet   string
	Source    entity.MemorySource
}

var tokenPattern = regexp.MustCompile(`[A-Za-z0-9_]+`)

// BuildFTSQuery converts a raw query string into an FTS5 AND query.
// Returns empty string if no valid tokens are found.
// Matches OpenClaw's buildFtsQuery.
func BuildFTSQuery(raw string) string {
	tokens := tokenPattern.FindAllString(raw, -1)
	if len(tokens) == 0 {
		return ""
	}

	var cleaned []string
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		// Remove quotes from token for safety.
		t = strings.ReplaceAll(t, `"`, "")
		if t != "" {
			cleaned = append(cleaned, `"`+t+`"`)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	return strings.Join(cleaned, " AND ")
}

// BM25RankToScore converts a BM25 rank value to a 0-1 score.
// Matches OpenClaw's bm25RankToScore.
func BM25RankToScore(rank float64) float64 {
	normalized := rank
	if !math.IsInf(rank, 0) && !math.IsNaN(rank) {
		if rank < 0 {
			normalized = 0
		}
	} else {
		normalized = 999
	}
	return 1.0 / (1.0 + normalized)
}

// MergeParams holds all parameters for the hybrid merge operation.
type MergeParams struct {
	Vector        []VectorResult
	Keyword       []KeywordResult
	VectorWeight  float64
	TextWeight    float64
	MMR           MMRConfig
	TemporalDecay TemporalDecayConfig
	WorkspaceDir  string
	Now           time.Time
}

// MergeResults merges vector and keyword search results using weighted scoring,
// then applies temporal decay and MMR re-ranking.
//
// Pipeline: merge → weighted score → temporal decay → sort → MMR → final results.
// Matches OpenClaw's mergeHybridResults.
func MergeResults(params MergeParams) []entity.MemorySearchResult {
	type merged struct {
		id          string
		path        string
		startLine   int
		endLine     int
		source      entity.MemorySource
		snippet     string
		vectorScore float64
		textScore   float64
	}

	byID := make(map[string]*merged)

	for _, r := range params.Vector {
		byID[r.ID] = &merged{
			id:          r.ID,
			path:        r.Path,
			startLine:   r.StartLine,
			endLine:     r.EndLine,
			source:      r.Source,
			snippet:     r.Snippet,
			vectorScore: r.VectorScore,
		}
	}

	for _, r := range params.Keyword {
		if existing, ok := byID[r.ID]; ok {
			existing.textScore = r.TextScore
			if r.Snippet != "" {
				existing.snippet = r.Snippet
			}
		} else {
			byID[r.ID] = &merged{
				id:        r.ID,
				path:      r.Path,
				startLine: r.StartLine,
				endLine:   r.EndLine,
				source:    r.Source,
				snippet:   r.Snippet,
				textScore: r.TextScore,
			}
		}
	}

	// Build intermediate HybridResult slice with weighted scores.
	hybridResults := make([]HybridResult, 0, len(byID))
	for _, entry := range byID {
		score := params.VectorWeight*entry.vectorScore + params.TextWeight*entry.textScore
		hybridResults = append(hybridResults, HybridResult{
			Path:      entry.path,
			StartLine: entry.startLine,
			EndLine:   entry.endLine,
			Score:     score,
			Snippet:   entry.snippet,
			Source:    entry.source,
		})
	}

	// Apply temporal decay.
	hybridResults = ApplyTemporalDecay(hybridResults, params.TemporalDecay, params.WorkspaceDir, params.Now)

	// Sort by decayed score.
	sort.Slice(hybridResults, func(i, j int) bool {
		return hybridResults[i].Score > hybridResults[j].Score
	})

	// Apply MMR re-ranking if enabled.
	hybridResults = ApplyMMR(hybridResults, params.MMR)

	// Convert to final results.
	results := make([]entity.MemorySearchResult, len(hybridResults))
	for i, hr := range hybridResults {
		results[i] = entity.MemorySearchResult{
			Path:      hr.Path,
			StartLine: hr.StartLine,
			EndLine:   hr.EndLine,
			Score:     hr.Score,
			Snippet:   hr.Snippet,
			Source:    hr.Source,
		}
	}

	return results
}
