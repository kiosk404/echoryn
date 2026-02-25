package boltdb

import (
	"context"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/pkg/errno"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// SubAgentStore is a BoltDB implementation of service.SubAgentRegistry.
type SubAgentStore struct {
	db *bolt.DB
}

// NewSubAgentStore creates a new BoltDB-backed SubAgentStore.
func NewSubAgentStore(db *DB) *SubAgentStore {
	return &SubAgentStore{db: db.Bolt()}
}

func (s *SubAgentStore) Save(_ context.Context, record *entity.SubAgentRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSubAgents)
		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("marshal subagent record: %w", err)
		}
		return b.Put([]byte(record.ID), data)
	})
}

func (s *SubAgentStore) Get(_ context.Context, id string) (*entity.SubAgentRecord, error) {
	var record entity.SubAgentRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSubAgents)
		data := b.Get([]byte(id))
		if data == nil {
			return errno.ErrSubAgentNotFound
		}
		return json.Unmarshal(data, &record)
	})
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *SubAgentStore) ListByParent(_ context.Context, parentSessionID string) ([]*entity.SubAgentRecord, error) {
	var result []*entity.SubAgentRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSubAgents)
		return b.ForEach(func(_, v []byte) error {
			var record entity.SubAgentRecord
			if err := json.Unmarshal(v, &record); err != nil {
				return err
			}
			if record.ParentSessionID == parentSessionID {
				result = append(result, &record)
			}
			return nil
		})
	})
	return result, err
}

func (s *SubAgentStore) ListNonTerminal(_ context.Context) ([]*entity.SubAgentRecord, error) {
	var result []*entity.SubAgentRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSubAgents)
		return b.ForEach(func(_, v []byte) error {
			var record entity.SubAgentRecord
			if err := json.Unmarshal(v, &record); err != nil {
				return err
			}
			if !record.Status.IsTerminal() {
				result = append(result, &record)
			}
			return nil
		})
	})
	return result, err
}

func (s *SubAgentStore) Delete(_ context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSubAgents).Delete([]byte(id))
	})
}
