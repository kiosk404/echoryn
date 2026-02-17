package search

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/entity"
	meminternal "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/internal"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/internal/hybrid"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/store"
)

const (
	SnippetMaxChars = 700
)

type SearchVectorParams struct {
	DB            *sql.DB
	ProviderModel string
	QueryVec      []float32
	Limit         int
	SourceFilter  []entity.MemorySource
}

// SearchVector performs a vector similarity search against the chunks table.
// Currently, uses pure Go cosine similarity. (no sqlite-vec extension required)
func SearchVector(params SearchVectorParams) ([]hybrid.VectorResult, error) {
	if len(params.QueryVec) == 0 || params.Limit <= 0 {
		return nil, nil
	}

	// Load all chunks and compute cosine similarity in Go.
	chunks, err := listChunks(params.DB, params.ProviderModel, params.SourceFilter)
	if err != nil {
		return nil, err
	}

	type scored struct {
		chunk chunkRow
		score float64
	}

	var scoredResults []scored
	for _, chunk := range chunks {
		s := meminternal.CosineSimilarity(params.QueryVec, chunk.embedding)
		if s > 0 {
			scoredResults = append(scoredResults, scored{chunk: chunk, score: s})
		}
	}

	sort.Slice(scoredResults, func(i, j int) bool {
		return scoredResults[i].score > scoredResults[j].score
	})

	limit := params.Limit
	if limit > len(scoredResults) {
		limit = len(scoredResults)
	}

	results := make([]hybrid.VectorResult, 0, limit)
	for _, entry := range scoredResults[:limit] {
		results = append(results, hybrid.VectorResult{
			ID:          entry.chunk.id,
			Path:        entry.chunk.path,
			StartLine:   entry.chunk.startLine,
			EndLine:     entry.chunk.endLine,
			VectorScore: entry.score,
			Snippet:     meminternal.TruncateUTF8Safe(entry.chunk.text, SnippetMaxChars),
			Source:      entry.chunk.source,
		})
	}
	return results, nil
}

// SearchVectorVec performs a KNN vector search using the sqlite-vec (vec0) virtual table.
// This is much faster than brute-force cosine similarity for large datasets.
// It queries vec0 for nearest neighbors, then joins with chunks table for metadata.
func SearchVectorVec(params SearchVectorVecParams) ([]hybrid.VectorResult, error) {
	if len(params.QueryVec) == 0 || params.Limit <= 0 {
		return nil, nil
	}

	// Query vec0 for nearest chunk IDs.
	vecResults, err := store.SearchVec(params.DB, params.QueryVec, params.Limit*2) // over-fetch for source filtering
	if err != nil {
		return nil, err
	}

	if len(vecResults) == 0 {
		return nil, nil
	}

	// Build source filter set.
	sourceSet := make(map[string]struct{})
	for _, s := range params.SourceFilter {
		sourceSet[string(s)] = struct{}{}
	}

	// Look up chunk metadata and build results.
	var results []hybrid.VectorResult
	for _, vr := range vecResults {
		row := params.DB.QueryRow(
			`SELECT id, path, source, start_line, end_line, text FROM `+store.TableChunks+` WHERE id = ?`,
			vr.ChunkID,
		)
		var id, path, source, text string
		var startLine, endLine int
		if err := row.Scan(&id, &path, &source, &startLine, &endLine, &text); err != nil {
			continue
		}

		// Apply source filter.
		if len(sourceSet) > 0 {
			if _, ok := sourceSet[source]; !ok {
				continue
			}
		}

		// Convert distance to similarity score (vec0 returns L2 distance by default).
		// Score = 1 / (1 + distance) gives a 0-1 range where higher is better.
		score := 1.0 / (1.0 + vr.Distance)

		results = append(results, hybrid.VectorResult{
			ID:          id,
			Path:        path,
			StartLine:   startLine,
			EndLine:     endLine,
			VectorScore: score,
			Snippet:     meminternal.TruncateUTF8Safe(text, SnippetMaxChars),
			Source:      entity.MemorySource(source),
		})

		if len(results) >= params.Limit {
			break
		}
	}

	return results, nil
}

// SearchVectorVec performs a vector similarity search against the vec0 virtual table.
type SearchVectorVecParams struct {
	DB           *sql.DB
	QueryVec     []float32
	Limit        int
	SourceFilter []entity.MemorySource
}

// SearchKeywordParams holds the parameters for a keyword search.
type SearchKeywordParams struct {
	DB            *sql.DB
	ProviderModel string
	Query         string
	Limit         int
	SourceFilter  []entity.MemorySource
}

// SearchKeyword performs a keyword search using FTS5.
func SearchKeyword(params SearchKeywordParams) ([]hybrid.KeywordResult, error) {
	if params.Limit <= 0 {
		return nil, nil
	}

	ftsQuery := hybrid.BuildFTSQuery(params.Query)
	if ftsQuery == "" {
		return nil, nil
	}

	// Build source filter SQL.
	sourceSQL, sourceArgs := buildSourceFilter(params.SourceFilter)

	query := fmt.Sprintf(
		`SELECT id, path, source, start_line, end_line, text, bm25(%s) AS rank FROM %s WHERE %s MATCH ? AND model = ?%s ORDER BY rank ASC LIMIT ?`,
		store.TableChunksFTS, store.TableChunksFTS, store.TableChunksFTS, sourceSQL,
	)

	args := make([]interface{}, 0)
	args = append(args, ftsQuery, params.ProviderModel)
	args = append(args, sourceArgs...)
	args = append(args, params.Limit)

	rows, err := params.DB.Query(query, args...)
	if err != nil {
		return nil, nil // FTS may not be available, gracefully return empty
	}
	defer rows.Close()

	var results []hybrid.KeywordResult
	for rows.Next() {
		var id, path, source, text string
		var startLine, endLine int
		var rank float64
		if err := rows.Scan(&id, &path, &source, &startLine, &endLine, &text, &rank); err != nil {
			continue
		}
		textScore := hybrid.BM25RankToScore(rank)
		results = append(results, hybrid.KeywordResult{
			ID:        id,
			Path:      path,
			StartLine: startLine,
			EndLine:   endLine,
			TextScore: textScore,
			Snippet:   meminternal.TruncateUTF8Safe(text, SnippetMaxChars),
			Source:    entity.MemorySource(source),
		})
	}
	return results, nil
}

// chunkRow represents a stored chunk loaded from the database.
type chunkRow struct {
	id        string
	path      string
	startLine int
	endLine   int
	text      string
	embedding []float32
	source    entity.MemorySource
}

// listChunks loads all chunks from the database for a given model and source filter.
func listChunks(db *sql.DB, providerModel string, sourceFilter []entity.MemorySource) ([]chunkRow, error) {
	sourceSQL, sourceArgs := buildSourceFilter(sourceFilter)

	query := fmt.Sprintf(
		`SELECT id, path, start_line, end_line, text, embedding, source FROM %s WHERE model = ?%s`,
		store.TableChunks, sourceSQL,
	)

	args := make([]interface{}, 0)
	args = append(args, providerModel)
	args = append(args, sourceArgs...)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []chunkRow
	for rows.Next() {
		var id, path, text, embeddingStr, source string
		var startLine, endLine int
		if err := rows.Scan(&id, &path, &startLine, &endLine, &text, &embeddingStr, &source); err != nil {
			continue
		}
		embedding := meminternal.ParseEmbedding(embeddingStr)
		chunks = append(chunks, chunkRow{
			id:        id,
			path:      path,
			startLine: startLine,
			endLine:   endLine,
			text:      text,
			embedding: embedding,
			source:    entity.MemorySource(source),
		})
	}
	return chunks, nil
}

// buildSourceFilter generates the SQL clause and args for source filtering.
func buildSourceFilter(sources []entity.MemorySource) (string, []interface{}) {
	if len(sources) == 0 {
		return "", nil
	}
	if len(sources) == 1 {
		return " AND source = ?", []interface{}{string(sources[0])}
	}
	placeholders := make([]string, len(sources))
	args := make([]interface{}, len(sources))
	for i, s := range sources {
		placeholders[i] = "?"
		args[i] = string(s)
	}
	return fmt.Sprintf(" AND source IN (%s)", joinStrings(placeholders, ",")), args
}

func joinStrings(s []string, sep string) string {
	result := ""
	for i, v := range s {
		if i > 0 {
			result += sep
		}
		result += v
	}
	return result
}
