package boltdb

import (
	"context"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// RunStore is a BoltDB-backed store for agent runs.
type RunStore struct {
	db *bolt.DB
}

// NewRunStore creates a new RunStore.
func NewRunStore(db *DB) *RunStore {
	return &RunStore{db: db.Bolt()}
}

func (s *RunStore) Create(ctx context.Context, run *entity.Run) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRunStore)
		data, err := json.Marshal(run)
		if err != nil {
			return fmt.Errorf("failed to marshal run: %w", err)
		}
		return b.Put([]byte(run.ID), data)
	})
}

func (s *RunStore) Get(ctx context.Context, id string) (*entity.Run, error) {
	var run entity.Run
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRunStore)
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("run %q not found", id)
		}
		return json.Unmarshal(data, &run)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get run %q: %w", id, err)
	}
	return &run, nil
}

func (s *RunStore) Update(ctx context.Context, run *entity.Run) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRunStore)
		if b.Get([]byte(run.ID)) == nil {
			return fmt.Errorf("run %q not found", run.ID)
		}
		data, err := json.Marshal(run)
		if err != nil {
			return fmt.Errorf("failed to marshal run: %w", err)
		}
		return b.Put([]byte(run.ID), data)
	})
}

func (s *RunStore) ListBySession(_ context.Context, sessionID string) ([]*entity.Run, error) {
	var runs []*entity.Run
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRunStore)
		return b.ForEach(func(k, v []byte) error {
			var r entity.Run
			if err := json.Unmarshal(v, &r); err != nil {
				return fmt.Errorf("failed to unmarshal run: %w", err)
			}
			if r.SessionID == sessionID {
				runs = append(runs, &r)
			}
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list runs by session %q: %w", sessionID, err)
	}
	return runs, nil
}
