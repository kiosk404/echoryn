package subagent

import (
	"context"
	"strconv"
	"strings"

	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// FindByIdentifier performs multi-dimensional fuzzy lookup on sub-agent records.
//
// Aligned with OpenClaw's flexible subagent identification which supports:
//   - Numeric index: "0", "1", "2" → match by spawn order (SubAgentRecord.Index)
//   - "subagent-N": "subagent-0", "subagent-2" → extract N and match by Index
//   - Label prefix: "poem-writer" → match by Label prefix (case-insensitive)
//   - ID prefix: "abc123" → match by ID prefix
//   - Exact ID: full UUID match
//
// Returns the first matching record from the given parent's children, or nil.
func FindByIdentifier(ctx context.Context, registry Registry, parentSessionID, identifier string) (*entity.SubAgentRecord, error) {
	records, err := registry.ListByParent(ctx, parentSessionID)
	if err != nil {
		logger.Warn("[subagent] FindByIdentifier: ListByParent failed for parentSessionID=%s: %v", parentSessionID, err)
		return nil, err
	}
	logger.Info("[subagent] FindByIdentifier: parentSessionID=%s, identifier=%q, found %d children", parentSessionID, identifier, len(records))
	match := matchByIdentifier(records, identifier)
	if match == nil {
		logger.Warn("[subagent] FindByIdentifier: no match for identifier=%q among %d children of parentSessionID=%s",
			identifier, len(records), parentSessionID)
	} else {
		logger.Info("[subagent] FindByIdentifier: matched record id=%s, index=%d, label=%q for identifier=%q",
			match.ID, match.Index, match.Label, identifier)
	}
	return match, nil
}

// matchByIdentifier implements the multi-dimensional matching logic.
//
// Priority order: exact ID → exact label → "subagent-N" by label → pure numeric by index → label prefix → ID prefix.
//
// IMPORTANT: When identifier is "subagent-N", we match by LABEL first (exact match),
// NOT by index. This is because labels like "subagent-1", "subagent-2" are assigned
// during spawn and are what the LLM sees in tool results.
func matchByIdentifier(records []*entity.SubAgentRecord, identifier string) *entity.SubAgentRecord {
	if len(records) == 0 || identifier == "" {
		return nil
	}

	identifier = strings.TrimSpace(identifier)

	// 1. Exact ID match (highest priority).
	for _, r := range records {
		if r.ID == identifier {
			return r
		}
	}

	// 2. Exact label match (case-insensitive).
	lower := strings.ToLower(identifier)
	for _, r := range records {
		if r.Label != "" && strings.ToLower(r.Label) == lower {
			return r
		}
	}

	// 3. Pure numeric → match by Index.
	if idx, err := strconv.Atoi(identifier); err == nil {
		for _, r := range records {
			if r.Index == idx {
				return r
			}
		}
	}

	// 4. Label prefix match (case-insensitive, non-exact).
	for _, r := range records {
		if r.Label != "" && strings.HasPrefix(strings.ToLower(r.Label), lower) {
			return r
		}
	}

	// 5. ID prefix match (fallback).
	for _, r := range records {
		if strings.HasPrefix(r.ID, identifier) {
			return r
		}
	}

	return nil
}
