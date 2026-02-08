package scheduler

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type NodeSelector interface {
}

// --------------------------------------------------------------------------
// ProfileProvider — data source abstraction
// --------------------------------------------------------------------------

// ProfileProvider abstracts the data source for Golem profiles. The scheduler
// uses this to obtain the current snapshot of all available Golem nodes and
// their capabilities, resources, and installed skills.
type ProfileProvider interface {
	// ListProfiles returns all Golem profiles that are currently online
	// or in a schedulable state.
	ListProfiles(ctx context.Context) ([]GolemProfile, error)

	// GetProfile returns the profile for a specific Golem node by ID.
	GetProfile(ctx context.Context, nodeID string) (*GolemProfile, error)
}

// --------------------------------------------------------------------------
// DirectSelector — explicit node targeting
// --------------------------------------------------------------------------

// DirectSelector implements NodeSelector for the DirectMode scheduling path.
// It validates that the explicitly targeted node exists, is healthy, and meets
// all hard constraints (capabilities, skills, features, resources).
type DirectSelector struct {
	provider ProfileProvider
}

// NewDirectSelector creates a DirectSelector backed by the given profile provider.
func NewDirectSelector(provider ProfileProvider) *DirectSelector {
	return &DirectSelector{provider: provider}
}

// Name returns the selector name.
func (s *DirectSelector) Name() string { return "direct" }

// Select validates and selects the explicitly targeted node.
func (s *DirectSelector) Select(ctx context.Context, req *ScheduleRequest, candidates []GolemProfile) (*ScheduleDecision, error) {
	start := time.Now()

	if req.TargetNodeID == "" {
		return nil, fmt.Errorf("scheduler: DirectSelector requires a non-empty TargetNodeID")
	}

	// Find the target among candidates.
	var target *GolemProfile
	for i := range candidates {
		if candidates[i].NodeInfo.ID == req.TargetNodeID {
			target = &candidates[i]
			break
		}
	}

	if target == nil {
		return nil, fmt.Errorf("scheduler: target node %q not found among %d candidates", req.TargetNodeID, len(candidates))
	}

	// Validate hard constraints.
	scorer := &constraintChecker{}
	if reason := scorer.check(req, target); reason != "" {
		return nil, fmt.Errorf("scheduler: target node %q rejected: %s", req.TargetNodeID, reason)
	}

	return &ScheduleDecision{
		Mode:           DirectMode,
		SelectedNodeID: target.NodeInfo.ID,
		Reason:         fmt.Sprintf("directly targeted node %q passed all constraints", req.TargetNodeID),
		CandidateCount: len(candidates),
		EligibleCount:  1,
		DecidedAt:      time.Now(),
		Latency:        time.Since(start),
	}, nil
}

// --------------------------------------------------------------------------
// AISelector — autonomous AI-driven node selection
// --------------------------------------------------------------------------

// ScoringWeights controls the relative importance of each scoring dimension.
// All weights should sum to 1.0 for normalised scoring, but this is not enforced.
type ScoringWeights struct {
	Capability float64
	Skill      float64
	Resource   float64
	Load       float64
	Tag        float64
	Affinity   float64
}

// DefaultScoringWeights returns a balanced set of weights.
func DefaultScoringWeights() ScoringWeights {
	return ScoringWeights{
		Capability: 0.25,
		Skill:      0.20,
		Resource:   0.20,
		Load:       0.20,
		Tag:        0.10,
		Affinity:   0.05,
	}
}

// AISelector implements NodeSelector for the AIMode scheduling path.
// It evaluates every candidate Golem against a multi-dimensional scoring model
// that considers capabilities, installed skills, system resources, current load,
// tag preferences, and affinity hints.
type AISelector struct {
	weights ScoringWeights
}

// NewAISelector creates an AISelector with the given scoring weights.
func NewAISelector(weights ScoringWeights) *AISelector {
	return &AISelector{weights: weights}
}

// NewDefaultAISelector creates an AISelector with default scoring weights.
func NewDefaultAISelector() *AISelector {
	return NewAISelector(DefaultScoringWeights())
}

// Name returns the selector name.
func (s *AISelector) Name() string { return "ai" }

// Select evaluates all candidates and returns the highest-scoring eligible node.
func (s *AISelector) Select(ctx context.Context, req *ScheduleRequest, candidates []GolemProfile) (*ScheduleDecision, error) {
	start := time.Now()

	if len(candidates) == 0 {
		return nil, fmt.Errorf("scheduler: AISelector received 0 candidates")
	}

	checker := &constraintChecker{}
	scores := make([]NodeScore, 0, len(candidates))
	var eligible []NodeScore

	for i := range candidates {
		profile := &candidates[i]
		ns := s.score(req, profile)

		// Hard-constraint check.
		if reason := checker.check(req, profile); reason != "" {
			ns.Eligible = false
			ns.RejectReason = reason
		} else {
			ns.Eligible = true
		}

		scores = append(scores, ns)
		if ns.Eligible {
			eligible = append(eligible, ns)
		}
	}

	if len(eligible) == 0 {
		return nil, fmt.Errorf("scheduler: no eligible Golem nodes among %d candidates", len(candidates))
	}

	// Sort eligible nodes by TotalScore descending.
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].TotalScore > eligible[j].TotalScore
	})

	best := eligible[0]

	return &ScheduleDecision{
		Mode:           AIMode,
		SelectedNodeID: best.NodeID,
		Reason:         s.buildReason(&best, len(eligible)),
		Scores:         scores,
		CandidateCount: len(candidates),
		EligibleCount:  len(eligible),
		DecidedAt:      time.Now(),
		Latency:        time.Since(start),
	}, nil
}

// score computes the multi-dimensional score for a single candidate.
func (s *AISelector) score(req *ScheduleRequest, profile *GolemProfile) NodeScore {
	ns := NodeScore{
		NodeID: profile.NodeInfo.ID,
	}

	ns.CapabilityScore = s.scoreCapabilities(req, profile)
	ns.SkillScore = s.scoreSkills(req, profile)
	ns.ResourceScore = s.scoreResources(req, profile)
	ns.LoadScore = s.scoreLoad(profile)
	ns.TagScore = s.scoreTags(req, profile)
	ns.AffinityScore = s.scoreAffinity(req, profile)

	// Weighted aggregate.
	ns.TotalScore = ns.CapabilityScore*s.weights.Capability +
		ns.SkillScore*s.weights.Skill +
		ns.ResourceScore*s.weights.Resource +
		ns.LoadScore*s.weights.Load +
		ns.TagScore*s.weights.Tag +
		ns.AffinityScore*s.weights.Affinity

	return ns
}

// scoreCapabilities returns 1.0 if all required capabilities are present, otherwise
// the fraction of matched capabilities.
func (s *AISelector) scoreCapabilities(req *ScheduleRequest, profile *GolemProfile) float64 {
	if len(req.RequiredCapabilities) == 0 {
		return 1.0
	}
	capSet := make(map[string]struct{}, len(profile.NodeInfo.Capabilities))
	for _, c := range profile.NodeInfo.Capabilities {
		capSet[c.Name] = struct{}{}
	}
	matched := 0
	for _, rc := range req.RequiredCapabilities {
		if _, ok := capSet[rc]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(req.RequiredCapabilities))
}

// scoreSkills returns the fraction of required skills that are installed.
func (s *AISelector) scoreSkills(req *ScheduleRequest, profile *GolemProfile) float64 {
	if len(req.RequiredSkills) == 0 {
		return 1.0
	}
	skillSet := make(map[string]struct{}, len(profile.InstalledSkills))
	for _, sk := range profile.InstalledSkills {
		skillSet[sk.ID] = struct{}{}
		skillSet[sk.Name] = struct{}{}
	}
	matched := 0
	for _, rs := range req.RequiredSkills {
		if _, ok := skillSet[rs]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(req.RequiredSkills))
}

// scoreResources evaluates available system resources (higher is better).
func (s *AISelector) scoreResources(_ *ScheduleRequest, profile *GolemProfile) float64 {
	info := profile.NodeInfo.SystemInfo
	load := profile.Load

	// Normalise individual dimensions to [0, 1].
	cpuScore := 1.0 - clamp(load.CPUPercent/100.0, 0, 1)
	memScore := 1.0 - clamp(load.MemoryPercent/100.0, 0, 1)
	diskScore := clamp(float64(info.DiskFreeMB)/10240.0, 0, 1) // 10 GB = 1.0

	return (cpuScore + memScore + diskScore) / 3.0
}

// scoreLoad evaluates how busy the node is (fewer tasks = higher score).
func (s *AISelector) scoreLoad(profile *GolemProfile) float64 {
	active := profile.Load.ActiveTasks
	queued := profile.Load.QueuedTasks
	total := active + queued
	if total == 0 {
		return 1.0
	}
	// Exponential decay: score drops as total tasks increase.
	return math.Exp(-0.3 * float64(total))
}

// scoreTags returns the fraction of preferred tags that match.
func (s *AISelector) scoreTags(req *ScheduleRequest, profile *GolemProfile) float64 {
	if len(req.PreferredTags) == 0 {
		return 1.0
	}
	matched := 0
	for k, v := range req.PreferredTags {
		if profile.Tags[k] == v {
			matched++
		}
	}
	return float64(matched) / float64(len(req.PreferredTags))
}

// scoreAffinity returns a score based on affinity / anti-affinity hints.
func (s *AISelector) scoreAffinity(req *ScheduleRequest, profile *GolemProfile) float64 {
	if req.Hints == nil {
		return 0.5 // neutral
	}
	nodeID := profile.NodeInfo.ID

	// Anti-affinity penalty.
	for _, anti := range req.Hints.AntiAffinity {
		if anti == nodeID {
			return 0.0
		}
	}

	// Affinity bonus.
	if req.Hints.Affinity != "" && req.Hints.Affinity == nodeID {
		return 1.0
	}

	return 0.5
}

// buildReason produces a human-readable explanation of the AI selection.
func (s *AISelector) buildReason(best *NodeScore, eligibleCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "selected node %q (score=%.3f) from %d eligible candidates; ", best.NodeID, best.TotalScore, eligibleCount)
	fmt.Fprintf(&b, "breakdown: capability=%.2f, skill=%.2f, resource=%.2f, load=%.2f, tag=%.2f, affinity=%.2f",
		best.CapabilityScore, best.SkillScore, best.ResourceScore, best.LoadScore, best.TagScore, best.AffinityScore)
	return b.String()
}

// --------------------------------------------------------------------------
// CompositeSelector — Chain of Responsibility
// --------------------------------------------------------------------------

// CompositeSelector chains multiple NodeSelector implementations.
// The first selector to return a successful decision wins.
// This enables layered strategies: e.g., try direct first, fallback to AI.
type CompositeSelector struct {
	selectors []NodeSelector
}

// NewCompositeSelector creates a selector that tries each strategy in order.
func NewCompositeSelector(selectors ...NodeSelector) *CompositeSelector {
	return &CompositeSelector{selectors: selectors}
}

// Name returns a composite name.
func (s *CompositeSelector) Name() string {
	names := make([]string, len(s.selectors))
	for i, sel := range s.selectors {
		names[i] = sel.Name()
	}
	return "composite[" + strings.Join(names, "→") + "]"
}

// Select tries each selector in order, returning the first successful decision.
func (s *CompositeSelector) Select(ctx context.Context, req *ScheduleRequest, candidates []GolemProfile) (*ScheduleDecision, error) {
	var lastErr error
	for _, sel := range s.selectors {
		decision, err := sel.Select(ctx, req, candidates)
		if err == nil {
			return decision, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("scheduler: all %d selectors failed, last error: %w", len(s.selectors), lastErr)
}

// --------------------------------------------------------------------------
// FilterSelector — pre-filter decorator
// --------------------------------------------------------------------------

// NodeFilter is a predicate that decides whether a GolemProfile is eligible.
type NodeFilter func(profile *GolemProfile) bool

// FilterSelector wraps another NodeSelector, pre-filtering candidates before
// delegating to the inner selector. Implements the Decorator pattern.
type FilterSelector struct {
	inner   NodeSelector
	filters []NodeFilter
}

// NewFilterSelector creates a filtering decorator around the given selector.
func NewFilterSelector(inner NodeSelector, filters ...NodeFilter) *FilterSelector {
	return &FilterSelector{inner: inner, filters: filters}
}

// Name returns the decorated name.
func (s *FilterSelector) Name() string {
	return "filtered(" + s.inner.Name() + ")"
}

// Select applies all filters and delegates to the inner selector.
func (s *FilterSelector) Select(ctx context.Context, req *ScheduleRequest, candidates []GolemProfile) (*ScheduleDecision, error) {
	filtered := make([]GolemProfile, 0, len(candidates))
	for i := range candidates {
		keep := true
		for _, f := range s.filters {
			if !f(&candidates[i]) {
				keep = false
				break
			}
		}
		if keep {
			filtered = append(filtered, candidates[i])
		}
	}
	return s.inner.Select(ctx, req, filtered)
}

// --------------------------------------------------------------------------
// Built-in NodeFilters
// --------------------------------------------------------------------------

// OnlineFilter returns a NodeFilter that only keeps online nodes.
func OnlineFilter() NodeFilter {
	return func(profile *GolemProfile) bool {
		return profile.NodeInfo.Status == "online"
	}
}

// HealthyFilter returns a NodeFilter that only keeps nodes above the given health threshold.
func HealthyFilter(minHealth float64) NodeFilter {
	return func(profile *GolemProfile) bool {
		return profile.HealthScore >= minHealth
	}
}

// FeatureFilter returns a NodeFilter that only keeps nodes supporting all given features.
func FeatureFilter(features ...string) NodeFilter {
	return func(profile *GolemProfile) bool {
		featureSet := make(map[string]struct{}, len(profile.SupportedFeatures))
		for _, f := range profile.SupportedFeatures {
			featureSet[f] = struct{}{}
		}
		for _, required := range features {
			if _, ok := featureSet[required]; !ok {
				return false
			}
		}
		return true
	}
}

// --------------------------------------------------------------------------
// constraintChecker — hard-constraint validation (shared helper)
// --------------------------------------------------------------------------

// constraintChecker validates that a GolemProfile meets all hard constraints
// specified in a ScheduleRequest. It is used by both DirectSelector and AISelector.
type constraintChecker struct{}

// check returns an empty string if the node passes all constraints, or a
// human-readable rejection reason.
func (c *constraintChecker) check(req *ScheduleRequest, profile *GolemProfile) string {
	// 1. Node must be online.
	if profile.NodeInfo.Status != "online" {
		return fmt.Sprintf("node status is %q, expected online", profile.NodeInfo.Status)
	}

	// 2. Required capabilities.
	if len(req.RequiredCapabilities) > 0 {
		capSet := make(map[string]struct{}, len(profile.NodeInfo.Capabilities))
		for _, c := range profile.NodeInfo.Capabilities {
			capSet[c.Name] = struct{}{}
		}
		for _, rc := range req.RequiredCapabilities {
			if _, ok := capSet[rc]; !ok {
				return fmt.Sprintf("missing required capability %q", rc)
			}
		}
	}

	// 3. Required skills.
	if len(req.RequiredSkills) > 0 {
		skillSet := make(map[string]struct{}, len(profile.InstalledSkills))
		for _, sk := range profile.InstalledSkills {
			skillSet[sk.ID] = struct{}{}
			skillSet[sk.Name] = struct{}{}
		}
		for _, rs := range req.RequiredSkills {
			if _, ok := skillSet[rs]; !ok {
				return fmt.Sprintf("missing required skill %q", rs)
			}
		}
	}

	// 4. Required features.
	if len(req.RequiredFeatures) > 0 {
		featureSet := make(map[string]struct{}, len(profile.SupportedFeatures))
		for _, f := range profile.SupportedFeatures {
			featureSet[f] = struct{}{}
		}
		for _, rf := range req.RequiredFeatures {
			if _, ok := featureSet[rf]; !ok {
				return fmt.Sprintf("missing required feature %q", rf)
			}
		}
	}

	// 5. Resource requirements.
	if rr := req.ResourceRequirements; rr != nil {
		info := profile.NodeInfo.SystemInfo
		load := profile.Load

		if rr.MinCPUCores > 0 && info.CPUCores < rr.MinCPUCores {
			return fmt.Sprintf("insufficient CPU cores: have %d, need %d", info.CPUCores, rr.MinCPUCores)
		}
		if rr.MinMemoryMB > 0 && int64(info.MemoryMB) < rr.MinMemoryMB {
			return fmt.Sprintf("insufficient memory: have %dMB, need %dMB", info.MemoryMB, rr.MinMemoryMB)
		}
		if rr.MinDiskFreeMB > 0 && int64(info.DiskFreeMB) < rr.MinDiskFreeMB {
			return fmt.Sprintf("insufficient disk: have %dMB, need %dMB", info.DiskFreeMB, rr.MinDiskFreeMB)
		}
		if rr.MaxCPUPercent > 0 && load.CPUPercent > rr.MaxCPUPercent {
			return fmt.Sprintf("CPU usage too high: %.1f%% > %.1f%%", load.CPUPercent, rr.MaxCPUPercent)
		}
		if rr.MaxMemoryPercent > 0 && load.MemoryPercent > rr.MaxMemoryPercent {
			return fmt.Sprintf("memory usage too high: %.1f%% > %.1f%%", load.MemoryPercent, rr.MaxMemoryPercent)
		}
		if rr.MaxActiveTasks > 0 && load.ActiveTasks > rr.MaxActiveTasks {
			return fmt.Sprintf("too many active tasks: %d > %d", load.ActiveTasks, rr.MaxActiveTasks)
		}
	}

	return ""
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
