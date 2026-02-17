package hybrid

import (
	"math"
	"regexp"
	"sort"
	"strings"

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

// MergeResults merges vector and keyword search results using weighted scoring.
// Matches OpenClaw's mergeHybridResults.
func MergeResults(vector []VectorResult, keyword []KeywordResult, vectorWeight, textWeight float64) []entity.MemorySearchResult {
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

	for _, r := range vector {
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

	for _, r := range keyword {
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

	results := make([]entity.MemorySearchResult, 0, len(byID))
	for _, entry := range byID {
		score := vectorWeight*entry.vectorScore + textWeight*entry.textScore
		results = append(results, entity.MemorySearchResult{
			Path:      entry.path,
			StartLine: entry.startLine,
			EndLine:   entry.endLine,
			Score:     score,
			Snippet:   entry.snippet,
			Source:    entry.source,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}
