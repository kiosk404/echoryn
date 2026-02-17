package store

import (
	"database/sql"
	"fmt"
)

const (
	TableMeta           = "meta"
	TableFiles          = "files"
	TableChunks         = "chunks"
	TableEmbeddingCache = "embedding_cache"
	TableChunksFTS      = "chunks_fts"
	TableChunksVec      = "chunks_vec"

	// Meta keys.
	MetaKeyProvider = "provider"
	MetaKeyModel    = "model"
)

// SchemaResult holds the outcome of schema initialization.
type SchemaResult struct {
	// FTSAvailable indicates whether FTS5 was successfully created.
	FTSAvailable bool

	// FTSError is the error message if FTS5 creation failed.
	FTSError string

	// VecAvailable indicates whether vector index was successfully created.
	VecAvailable bool

	// VecError is the error message if vector index creation failed.
	VecError string
}

// EnsureSchema creates all required tables and indexes.
// Matches OpenClaw's ensureMemoryIndexSchema.
func EnsureSchema(db *sql.DB, ftsEnabled bool, vecConfig *VecSchemaConfig) (*SchemaResult, error) {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ` + TableMeta + ` (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + TableFiles + ` (
			path TEXT PRIMARY KEY,
			source TEXT NOT NULL DEFAULT 'memory',
			hash TEXT NOT NULL,
			mtime INTEGER NOT NULL,
			size INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + TableChunks + ` (
			id TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'memory',
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			hash TEXT NOT NULL,
			model TEXT NOT NULL,
			text TEXT NOT NULL,
			embedding TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + TableEmbeddingCache + ` (
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			provider_key TEXT NOT NULL,
			hash TEXT NOT NULL,
			embedding TEXT NOT NULL,
			dims INTEGER,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (provider, model, provider_key, hash)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_embedding_cache_updated_at ON ` + TableEmbeddingCache + `(updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_path ON ` + TableChunks + `(path)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_source ON ` + TableChunks + `(source)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("exec schema: %w", err)
		}
	}

	// Ensure columns exist (migration support).
	ensureColumn(db, TableFiles, "source", "TEXT NOT NULL DEFAULT 'memory'")
	ensureColumn(db, TableChunks, "source", "TEXT NOT NULL DEFAULT 'memory'")

	result := &SchemaResult{}
	if ftsEnabled {
		ftsSQL := `CREATE VIRTUAL TABLE IF NOT EXISTS ` + TableChunksFTS + ` USING fts5(
			text,
			id UNINDEXED,
			path UNINDEXED,
			source UNINDEXED,
			model UNINDEXED,
			start_line UNINDEXED,
			end_line UNINDEXED
		)`
		if _, err := db.Exec(ftsSQL); err != nil {
			result.FTSError = err.Error()
		} else {
			result.FTSAvailable = true
		}
	}

	// Create sqlite-vec virtual table if requested and the extension is loaded.
	if vecConfig != nil && vecConfig.Enabled && vecConfig.Dimensions > 0 {
		if vecConfig.ExtensionPath != "" {
			_, _ = db.Exec("SELECT load_extension(?)", vecConfig.ExtensionPath)
		}
		vecSQL := fmt.Sprintf(
			`CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(chunk_id TEXT PRIMARY KEY,
			embedding float[%d])`,
			TableChunksVec, vecConfig.Dimensions)
		if _, err := db.Exec(vecSQL); err != nil {
			result.VecError = err.Error()
		} else {
			result.VecAvailable = true
		}
	}

	return result, nil
}

// VecSchemaConfig holds configuration for sqlite-vector index creation.
type VecSchemaConfig struct {
	// Enabled indicates whether to create the vector index.
	Enabled bool

	// Dimensions is the number of dimensions for the vector index.
	Dimensions int

	// ExtensionPath is the path to the sqlite-vec extension.
	ExtensionPath string
}

// InsertVecChunk inserts a chunk embedding into the vec0 virtual table.
func InsertVecChunk(db *sql.DB, chunkID string, embedding []float32) error {
	if len(embedding) == 0 {
		return nil
	}
	// sqlite-vec expects the embedding as a JSON array or binary blob.
	// Using JSON array format for compatibility.
	vecJSON := float32SliceToJSON(embedding)
	_, err := db.Exec(
		`INSERT OR REPLACE INTO `+TableChunksVec+` (chunk_id, embedding) VALUES (?, ?)`,
		chunkID, vecJSON,
	)
	return err
}

// DeleteVecChunksByPath deletes vec entries for chunks belonging to a file path.
func DeleteVecChunksByPath(db *sql.DB, path string, source string) {
	db.Exec(
		`DELETE FROM `+TableChunksVec+` WHERE chunk_id IN (SELECT id FROM `+TableChunks+` WHERE path = ? AND source = ?)`,
		path, source,
	)
}

// SearchVec performs a KNN vector search using the vec0 virtual table.
// Returns chunk IDs and distances, sorted by nearest first.
func SearchVec(db *sql.DB, queryVec []float32, limit int) ([]VecSearchResult, error) {
	if len(queryVec) == 0 || limit <= 0 {
		return nil, nil
	}

	vecJSON := float32SliceToJSON(queryVec)

	rows, err := db.Query(
		`SELECT chunk_id, distance FROM `+TableChunksVec+` WHERE embedding MATCH ? ORDER BY distance LIMIT ?`,
		vecJSON, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []VecSearchResult
	for rows.Next() {
		var r VecSearchResult
		if err := rows.Scan(&r.ChunkID, &r.Distance); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// VecSearchResult holds a single result from vec0 KNN search.
type VecSearchResult struct {
	ChunkID  string
	Distance float64
}

// float32SliceToJSON converts a float32 slice to a JSON array string.
func float32SliceToJSON(v []float32) string {
	buf := make([]byte, 0, len(v)*10+2)
	buf = append(buf, '[')
	for i, f := range v {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, fmt.Sprintf("%g", f)...)
	}
	buf = append(buf, ']')
	return string(buf)
}

// ensureColumn adds a column to an existing table if it doesn't already exist.
func ensureColumn(db *sql.DB, table, column, definition string) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == column {
			return // column already exists
		}
	}

	db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
}
