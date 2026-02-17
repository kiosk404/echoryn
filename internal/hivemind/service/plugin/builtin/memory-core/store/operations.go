package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory-core/entity"
)

// GetFileRecord retrieves the hash of a file from the database.
func GetFileRecord(db *sql.DB, path string, source entity.MemorySource) (hash string, found bool, err error) {
	row := db.QueryRow(
		`SELECT hash FROM `+TableFiles+` WHERE path = ? AND source = ?`,
		path, string(source))
	if err = row.Scan(&hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return hash, true, err
}

// UpsertFileRecord inserts or updates a file record in the database.
func UpsertFileRecord(db *sql.DB, entry *entity.MemoryFileEntry, source entity.MemorySource) (err error) {
	_, err = db.Exec(
		`INSERT OR REPLACE INTO `+TableFiles+` (path, source, hash, mtime, size) VALUES (?, ?, ?, ?, ?)`,
		entry.Path, string(source), entry.Hash, entry.MtimeMs, entry.Size)
	return err
}

// DeleteFileAndChunks deletes a file and its chunks from the database.
func DeleteFileAndChunks(db *sql.DB, path string, source entity.MemorySource, model string, ftsAvailable bool) (err error) {
	_, err = db.Exec(`DELETE FROM `+TableFiles+` WHERE path = ? AND source = ?`, path, string(source))
	_, err = db.Exec(`DELETE FROM `+TableChunksVec+` WHERE id IN (SELECT id FROM `+TableChunks+` WHERE path = ? AND source = ?)`, path, string(source))
	_, err = db.Exec(`DELETE FROM `+TableChunks+` WHERE path = ? AND source = ?`, path, string(source))

	if ftsAvailable {
		_, err = db.Exec(
			`DELETE FROM `+TableChunksFTS+` WHERE path = ? AND source = ? AND model = ?`,
			path, string(source), model)
	}
	return err
}

// InsertChunk inserts a chunk into the database.
func InsertChunk(db *sql.DB, chunkID, path string, source entity.MemorySource,
	startLine, endLine int, hash, model, text, embeddingJSON string) (err error) {
	_, err = db.Exec(
		`INSERT INTO `+TableChunks+` (id, path, source, start_line, end_line, hash, model, text, embedding, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		chunkID, path, string(source), startLine, endLine, hash, model, text, embeddingJSON, time.Now().UnixMilli())
	return err
}

// InsertFTSChunk inserts a chunk into the FTS table.
func InsertFTSChunk(db *sql.DB, text, chunkID, path string, source entity.MemorySource,
	model string, startLine, endLine int) (err error) {
	_, err = db.Exec(
		`INSERT INTO `+TableChunksFTS+` (text, id, path, source, model, start_line, end_line) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		text, chunkID, path, string(source), model, startLine, endLine)
	return err
}

// GetStalePaths returns a list of paths that have stale chunks.
func GetStalePaths(db *sql.DB, source entity.MemorySource) (paths []string, err error) {
	rows, err := db.Query(
		`SELECT path FROM `+TableChunks+` WHERE source = ?`, string(source))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// GetMeta retrieves a meta value by key
func GetMeta(db *sql.DB, key string) (string, error) {
	row := db.QueryRow(
		`SELECT value FROM `+TableMeta+` WHERE key = ?`, key)
	var value string
	err := row.Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// SetMeta sets a meta value by key
func SetMeta(db *sql.DB, key, value string) (err error) {
	_, err = db.Exec(
		`INSERT OR REPLACE INTO `+TableMeta+` (key, value) VALUES (?, ?)`,
		key, value)
	return err
}

// CountChunks returns the total number of chunks in the database.
func CountChunks(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM ` + TableChunks).Scan(&count)
	return count, err
}

// CountFiles returns the total number of indexed files.
func CountFiles(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM ` + TableFiles).Scan(&count)
	return count, err
}

// LoadEmbeddingCache retrieves cached embeddings for a set of chunk hashes.
func LoadEmbeddingCache(db *sql.DB, provider, model, providerKey string, hashes []string) (map[string]string, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	result := make(map[string]string)
	for _, h := range hashes {
		var embedding string
		err := db.QueryRow(
			`SELECT embedding FROM `+TableEmbeddingCache+` WHERE provider = ? AND model = ? AND provider_key = ? AND hash = ?`,
			provider, model, providerKey, h,
		).Scan(&embedding)
		if err == nil {
			result[h] = embedding
		}
	}
	return result, nil
}

// UpsertEmbeddingCache inserts or updates an embedding cache entry.
func UpsertEmbeddingCache(db *sql.DB, provider, model, providerKey, hash, embedding string, dims int) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO `+TableEmbeddingCache+` (provider, model, provider_key, hash, embedding, dims, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		provider, model, providerKey, hash, embedding, dims, time.Now().UnixMilli(),
	)
	return err
}

// PruneEmbeddingCache removes old entries beyond the max count.
func PruneEmbeddingCache(db *sql.DB, maxEntries int) error {
	if maxEntries <= 0 {
		return nil
	}
	_, err := db.Exec(
		fmt.Sprintf(
			`DELETE FROM %s WHERE rowid NOT IN (SELECT rowid FROM %s ORDER BY updated_at DESC LIMIT ?)`,
			TableEmbeddingCache, TableEmbeddingCache,
		),
		maxEntries,
	)
	return err
}
