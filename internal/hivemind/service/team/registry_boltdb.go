// Package team provides BoltDB-based persistent TeamRegistry.
package team

import (
	"context"
	"fmt"
	"time"

	"github.com/kiosk404/echoryn/pkg/utils/json"
	"go.etcd.io/bbolt"
)

// BoltDB bucket names.
var (
	bucketTemplates = []byte("team_templates")
	bucketTeams     = []byte("team_instances")
)

// BoltDBTeamRegistry implements TeamRegistry backed by BoltDB.
// It provides durable persistence for team templates and instances.
//
// Bucket layout:
//
//	team_templates/{id} → JSON(TeamTemplate)
//	team_instances/{id} → JSON(Team)
type BoltDBTeamRegistry struct {
	db *bbolt.DB
}

// NewBoltDBTeamRegistry creates a new BoltDB-based TeamRegistry.
// The dbPath is the path to the BoltDB file (will be created if not exists).
func NewBoltDBTeamRegistry(dbPath string) (*BoltDBTeamRegistry, error) {
	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open boltdb: %w", err)
	}

	// Ensure buckets exist.
	if err := db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketTemplates); err != nil {
			return fmt.Errorf("create templates bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists(bucketTeams); err != nil {
			return fmt.Errorf("create teams bucket: %w", err)
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, err
	}

	return &BoltDBTeamRegistry{db: db}, nil
}

// Close closes the BoltDB database.
func (r *BoltDBTeamRegistry) Close() error {
	return r.db.Close()
}

// --- Template operations ---

// SaveTemplate persists a team template.
func (r *BoltDBTeamRegistry) SaveTemplate(_ context.Context, template *TeamTemplate) error {
	if template.ID == "" {
		return fmt.Errorf("template ID is required")
	}

	data, err := json.Marshal(template)
	if err != nil {
		return fmt.Errorf("marshal template: %w", err)
	}

	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTemplates)
		return b.Put([]byte(template.ID), data)
	})
}

// GetTemplate retrieves a template by ID.
func (r *BoltDBTeamRegistry) GetTemplate(_ context.Context, templateID string) (*TeamTemplate, error) {
	var template *TeamTemplate

	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTemplates)
		data := b.Get([]byte(templateID))
		if data == nil {
			return nil // not found
		}

		template = &TeamTemplate{}
		return json.Unmarshal(data, template)
	})

	return template, err
}

// ListTemplates returns all templates matching the filter.
func (r *BoltDBTeamRegistry) ListTemplates(_ context.Context, filter *TemplateFilter) ([]*TeamTemplate, error) {
	var templates []*TeamTemplate

	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTemplates)
		return b.ForEach(func(_, v []byte) error {
			var t TeamTemplate
			if err := json.Unmarshal(v, &t); err != nil {
				return err
			}

			// Apply filter.
			if filter != nil {
				if filter.NamePrefix != "" && !hasPrefix(t.Name, filter.NamePrefix) {
					return nil
				}
				if len(filter.Tags) > 0 && !hasAnyTag(t.Tags, filter.Tags) {
					return nil
				}
			}

			templates = append(templates, &t)
			return nil
		})
	})

	return templates, err
}

// DeleteTemplate removes a template by ID.
func (r *BoltDBTeamRegistry) DeleteTemplate(_ context.Context, templateID string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTemplates)
		return b.Delete([]byte(templateID))
	})
}

// --- Team instance operations ---

// Save persists a team instance.
func (r *BoltDBTeamRegistry) Save(_ context.Context, team *Team) error {
	if team.ID == "" {
		return fmt.Errorf("team ID is required")
	}

	data, err := json.Marshal(team)
	if err != nil {
		return fmt.Errorf("marshal team: %w", err)
	}

	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTeams)
		return b.Put([]byte(team.ID), data)
	})
}

// Get retrieves a team by ID.
func (r *BoltDBTeamRegistry) Get(_ context.Context, teamID string) (*Team, error) {
	var team *Team

	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTeams)
		data := b.Get([]byte(teamID))
		if data == nil {
			return nil
		}

		team = &Team{}
		return json.Unmarshal(data, team)
	})

	return team, err
}

// ListByParent returns all teams created by the given parent session.
func (r *BoltDBTeamRegistry) ListByParent(_ context.Context, parentSessionID string) ([]*Team, error) {
	var teams []*Team

	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTeams)
		return b.ForEach(func(_, v []byte) error {
			var t Team
			if err := json.Unmarshal(v, &t); err != nil {
				return err
			}
			if t.ParentSessionID == parentSessionID {
				teams = append(teams, &t)
			}
			return nil
		})
	})

	return teams, err
}

// Delete removes a team by ID.
func (r *BoltDBTeamRegistry) Delete(_ context.Context, teamID string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTeams)
		return b.Delete([]byte(teamID))
	})
}

// --- Member operations ---

// AddMember adds a member to a team.
func (r *BoltDBTeamRegistry) AddMember(ctx context.Context, teamID string, member *TeamMember) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTeams)
		data := b.Get([]byte(teamID))
		if data == nil {
			return fmt.Errorf("team %s not found", teamID)
		}

		var team Team
		if err := json.Unmarshal(data, &team); err != nil {
			return err
		}

		team.Members = append(team.Members, member)
		team.UpdatedAt = time.Now()

		updated, err := json.Marshal(&team)
		if err != nil {
			return err
		}
		return b.Put([]byte(teamID), updated)
	})
}

// RemoveMember removes a member from a team.
func (r *BoltDBTeamRegistry) RemoveMember(ctx context.Context, teamID, memberID string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTeams)
		data := b.Get([]byte(teamID))
		if data == nil {
			return fmt.Errorf("team %s not found", teamID)
		}

		var team Team
		if err := json.Unmarshal(data, &team); err != nil {
			return err
		}

		// Remove the member.
		filtered := make([]*TeamMember, 0, len(team.Members))
		found := false
		for _, m := range team.Members {
			if m.ID == memberID {
				found = true
				continue
			}
			filtered = append(filtered, m)
		}
		if !found {
			return fmt.Errorf("member %s not found in team %s", memberID, teamID)
		}

		team.Members = filtered
		team.UpdatedAt = time.Now()

		updated, err := json.Marshal(&team)
		if err != nil {
			return err
		}
		return b.Put([]byte(teamID), updated)
	})
}

// UpdateMemberStatus updates a member's status.
func (r *BoltDBTeamRegistry) UpdateMemberStatus(ctx context.Context, teamID, memberID string, status TeamMemberStatus) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTeams)
		data := b.Get([]byte(teamID))
		if data == nil {
			return fmt.Errorf("team %s not found", teamID)
		}

		var team Team
		if err := json.Unmarshal(data, &team); err != nil {
			return err
		}

		found := false
		for _, m := range team.Members {
			if m.ID == memberID {
				m.Status = status
				if status.IsTerminal() {
					now := time.Now()
					m.CompletedAt = &now
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("member %s not found in team %s", memberID, teamID)
		}

		team.UpdatedAt = time.Now()

		updated, err := json.Marshal(&team)
		if err != nil {
			return err
		}
		return b.Put([]byte(teamID), updated)
	})
}

// --- Helper functions ---

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasAnyTag(tags []string, filter []string) bool {
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}
	for _, f := range filter {
		if tagSet[f] {
			return true
		}
	}
	return false
}

// --- Reverse Lookup ---

// FindBySessionID searches all teams for a member with the given session ID.
func (r *BoltDBTeamRegistry) FindBySessionID(_ context.Context, sessionID string) (*Team, *TeamMember, error) {
	var resultTeam *Team
	var resultMember *TeamMember

	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTeams)
		return b.ForEach(func(_, v []byte) error {
			if resultTeam != nil {
				return nil // already found
			}
			var t Team
			if err := json.Unmarshal(v, &t); err != nil {
				return nil // skip malformed entries
			}
			for _, m := range t.Members {
				if m.SessionID == sessionID {
					resultTeam = &t
					memberCopy := *m
					resultMember = &memberCopy
					return nil
				}
			}
			return nil
		})
	})

	return resultTeam, resultMember, err
}

// FindByWorkerRef searches all teams for a member with the given WorkerRef.
func (r *BoltDBTeamRegistry) FindByWorkerRef(_ context.Context, ref WorkerRef) (*Team, *TeamMember, error) {
	if ref.ID == "" {
		return nil, nil, nil
	}

	var resultTeam *Team
	var resultMember *TeamMember

	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTeams)
		return b.ForEach(func(_, v []byte) error {
			if resultTeam != nil {
				return nil
			}
			var t Team
			if err := json.Unmarshal(v, &t); err != nil {
				return nil
			}
			for _, m := range t.Members {
				if m.WorkerRef.ID == ref.ID {
					resultTeam = &t
					memberCopy := *m
					resultMember = &memberCopy
					return nil
				}
			}
			return nil
		})
	})

	return resultTeam, resultMember, err
}

// ListAll returns all active (non-dissolved) teams.
func (r *BoltDBTeamRegistry) ListAll(_ context.Context) ([]*Team, error) {
	var teams []*Team

	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTeams)
		return b.ForEach(func(_, v []byte) error {
			var t Team
			if err := json.Unmarshal(v, &t); err != nil {
				return nil
			}
			if !t.Status.IsTerminal() {
				teams = append(teams, &t)
			}
			return nil
		})
	})

	return teams, err
}

// Compile-time check.
var _ TeamRegistry = (*BoltDBTeamRegistry)(nil)
