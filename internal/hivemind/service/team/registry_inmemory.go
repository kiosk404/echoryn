package team

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// InMemoryTeamRegistry is an in-memory implementation of TeamRegistry.
// Suitable for development and testing. All data is lost on process restart.
type InMemoryTeamRegistry struct {
	mu        sync.RWMutex
	templates map[string]*TeamTemplate // templateID → template
	teams     map[string]*Team         // teamID → team
}

// NewInMemoryTeamRegistry creates an empty in-memory registry.
func NewInMemoryTeamRegistry() *InMemoryTeamRegistry {
	return &InMemoryTeamRegistry{
		templates: make(map[string]*TeamTemplate),
		teams:     make(map[string]*Team),
	}
}

var _ TeamRegistry = (*InMemoryTeamRegistry)(nil)

// --- Template Management ---

func (r *InMemoryTeamRegistry) SaveTemplate(_ context.Context, template *TeamTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if template.ID == "" {
		return fmt.Errorf("template ID is required")
	}

	// Deep copy to prevent external mutation.
	r.templates[template.ID] = r.copyTemplate(template)
	return nil
}

func (r *InMemoryTeamRegistry) GetTemplate(_ context.Context, templateID string) (*TeamTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.templates[templateID]
	if !ok {
		return nil, nil
	}
	return r.copyTemplate(t), nil
}

func (r *InMemoryTeamRegistry) ListTemplates(_ context.Context, filter *TemplateFilter) ([]*TeamTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*TeamTemplate
	for _, t := range r.templates {
		if r.matchesFilter(t, filter) {
			result = append(result, r.copyTemplate(t))
		}
	}
	return result, nil
}

func (r *InMemoryTeamRegistry) DeleteTemplate(_ context.Context, templateID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.templates, templateID)
	return nil
}

// --- Team Instance Management ---

func (r *InMemoryTeamRegistry) Save(_ context.Context, team *Team) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if team.ID == "" {
		return fmt.Errorf("team ID is required")
	}

	r.teams[team.ID] = r.copyTeam(team)
	return nil
}

func (r *InMemoryTeamRegistry) Get(_ context.Context, teamID string) (*Team, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.teams[teamID]
	if !ok {
		return nil, nil
	}
	return r.copyTeam(t), nil
}

func (r *InMemoryTeamRegistry) ListByParent(_ context.Context, parentSessionID string) ([]*Team, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Team
	for _, t := range r.teams {
		if t.ParentSessionID == parentSessionID {
			result = append(result, r.copyTeam(t))
		}
	}
	return result, nil
}

func (r *InMemoryTeamRegistry) Delete(_ context.Context, teamID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.teams, teamID)
	return nil
}

// --- Member Management ---

func (r *InMemoryTeamRegistry) AddMember(_ context.Context, teamID string, member *TeamMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	team, ok := r.teams[teamID]
	if !ok {
		return fmt.Errorf("team %s not found", teamID)
	}

	team.Members = append(team.Members, r.copyMember(member))
	return nil
}

func (r *InMemoryTeamRegistry) RemoveMember(_ context.Context, teamID string, memberID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	team, ok := r.teams[teamID]
	if !ok {
		return fmt.Errorf("team %s not found", teamID)
	}

	for i, m := range team.Members {
		if m.ID == memberID {
			team.Members = append(team.Members[:i], team.Members[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("member %s not found in team %s", memberID, teamID)
}

func (r *InMemoryTeamRegistry) UpdateMemberStatus(_ context.Context, teamID, memberID string, status TeamMemberStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	team, ok := r.teams[teamID]
	if !ok {
		return fmt.Errorf("team %s not found", teamID)
	}

	for _, m := range team.Members {
		if m.ID == memberID {
			m.Status = status
			return nil
		}
	}
	return fmt.Errorf("member %s not found in team %s", memberID, teamID)
}

// --- Filter helpers ---

func (r *InMemoryTeamRegistry) matchesFilter(t *TeamTemplate, filter *TemplateFilter) bool {
	if filter == nil {
		return true
	}

	// Name prefix filter.
	if filter.NamePrefix != "" && !strings.HasPrefix(strings.ToLower(t.Name), strings.ToLower(filter.NamePrefix)) {
		return false
	}

	// Tags filter (OR match).
	if len(filter.Tags) > 0 {
		matched := false
		for _, filterTag := range filter.Tags {
			for _, templateTag := range t.Tags {
				if strings.EqualFold(filterTag, templateTag) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// --- Deep copy helpers ---

func (r *InMemoryTeamRegistry) copyTemplate(t *TeamTemplate) *TeamTemplate {
	cp := *t
	if t.MemberSpecs != nil {
		cp.MemberSpecs = make([]*MemberSpec, len(t.MemberSpecs))
		for i, s := range t.MemberSpecs {
			specCopy := *s
			if s.Skills != nil {
				specCopy.Skills = make([]string, len(s.Skills))
				copy(specCopy.Skills, s.Skills)
			}
			cp.MemberSpecs[i] = &specCopy
		}
	}
	if t.Tags != nil {
		cp.Tags = make([]string, len(t.Tags))
		copy(cp.Tags, t.Tags)
	}
	return &cp
}

func (r *InMemoryTeamRegistry) copyTeam(t *Team) *Team {
	cp := *t
	if t.Members != nil {
		cp.Members = make([]*TeamMember, len(t.Members))
		for i, m := range t.Members {
			cp.Members[i] = r.copyMember(m)
		}
	}
	if t.Metadata != nil {
		cp.Metadata = make(map[string]string, len(t.Metadata))
		for k, v := range t.Metadata {
			cp.Metadata[k] = v
		}
	}
	if t.Result != nil {
		resultCopy := *t.Result
		if t.Result.MemberResults != nil {
			resultCopy.MemberResults = make(map[string]string, len(t.Result.MemberResults))
			for k, v := range t.Result.MemberResults {
				resultCopy.MemberResults[k] = v
			}
		}
		cp.Result = &resultCopy
	}
	return &cp
}

func (r *InMemoryTeamRegistry) copyMember(m *TeamMember) *TeamMember {
	cp := *m
	return &cp
}
