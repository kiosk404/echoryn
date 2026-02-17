package boltdb

import (
	"context"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// SessionStore implements the SessionStore interface using BoltDB.
type SessionStore struct {
	boltDB *bolt.DB
}

// NewSessionStore creates a new SessionStore instance.
func NewSessionStore(boltDB *DB) *SessionStore {
	return &SessionStore{boltDB: boltDB.Bolt()}
}

func (s *SessionStore) Create(_ context.Context, session *entity.Session) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionStore)
		data, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("failed to marshal session: %w", err)
		}
		return b.Put([]byte(session.ID), data)
	})
}

func (s *SessionStore) Get(_ context.Context, id string) (*entity.Session, error) {
	var session entity.Session
	err := s.boltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionStore)
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("session %q not found", id)
		}
		return json.Unmarshal(data, &session)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get session %q: %w", id, err)
	}
	return &session, nil
}

func (s *SessionStore) Update(_ context.Context, session *entity.Session) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionStore)
		if b.Get([]byte(session.ID)) == nil {
			return fmt.Errorf("session %q not found", session.ID)
		}
		data, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("failed to marshal session: %w", err)
		}
		return b.Put([]byte(session.ID), data)
	})
}

func (s *SessionStore) Delete(_ context.Context, id string) error {
	return s.boltDB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionStore)
		return b.Delete([]byte(id))
	})
}

func (s *SessionStore) ListByAgent(_ context.Context, agentID string) ([]*entity.Session, error) {
	var sessions []*entity.Session
	err := s.boltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionStore)
		return b.ForEach(func(k, v []byte) error {
			var session entity.Session
			if err := json.Unmarshal(v, &session); err != nil {
				return fmt.Errorf("failed to unmarshal session: %w", err)
			}
			if session.AgentID == agentID {
				sessions = append(sessions, &session)
			}
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions by agent %q: %w", agentID, err)
	}
	return sessions, nil
}
