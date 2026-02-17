package boltdb

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/boltdb/bolt"
)

var (
	bucketAgentStore   = []byte("agents")
	bucketSessionStore = []byte("sessions")
	bucketRunStore     = []byte("runs")
)

// DB wraps a BoltDB instance and manages its lifecycle.
type DB struct {
	db *bolt.DB
}

func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketAgentStore, bucketSessionStore, bucketRunStore} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return fmt.Errorf("failed to create bucket %q: %w", b, err)
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create buckets: %w", err)
	}
	return &DB{db: db}, nil
}

// Close closes the underlying BoltDB instance.
func (d *DB) Close() error {
	return d.db.Close()
}

// Bolt returns the underlying BoltDB instance.
func (d *DB) Bolt() *bolt.DB {
	return d.db
}
