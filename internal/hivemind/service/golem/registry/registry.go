package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
)

// InMemoryRegistry is an in-memory implementation of Registry.
type InMemoryRegistry struct {
	cfg      *Config
	mu       sync.RWMutex
	nodes    map[string]*NodeState // nodeID → NodeState
	enricher SkillsEnricher        // optional: enriches node skills from Hivemind-side catalog.

	cancel  context.CancelFunc
	stopped chan struct{}
}

var _ Registry = (*InMemoryRegistry)(nil)

// NewInMemoryRegistry creates a new in-memory Registry.
func NewInMemoryRegistry(cfg *Config) *InMemoryRegistry {
	return &InMemoryRegistry{
		cfg:     cfg,
		nodes:   make(map[string]*NodeState),
		stopped: make(chan struct{}),
	}
}

// SetSkillsEnricher sets the SkillsEnricher used to populate node skills on registration.
func (m *InMemoryRegistry) SetSkillsEnricher(enricher SkillsEnricher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enricher = enricher
}

func (m *InMemoryRegistry) RegisterNode(ctx context.Context, info *pb.NodeInfo, load *pb.NodeLoadInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.nodes) >= m.cfg.MaxNodes {
		return fmt.Errorf("maximum number of nodes (%d) reached", m.cfg.MaxNodes)
	}

	// Merge Hivemind-side global skills into the node's InstalledSkills.
	// Golem-reported skills take precedence (by name) over Hivemind-side ones.
	logger.Info("[Registry] RegisterNode: incoming InstalledSkills count=%d, enricher=%v",
		len(info.InstalledSkills), m.enricher != nil)
	for i, sk := range info.InstalledSkills {
		logger.Info("[Registry] RegisterNode: incoming skill[%d]: name=%q desc=%s",
			i, sk.Name, sk.Description[:min(len(sk.Description), 50)])
	}
	installedSkills := info.InstalledSkills
	if m.enricher != nil {
		globalSkills := m.enricher.GetGlobalSkills()
		logger.Info("[Registry] RegisterNode: enricher has %d global skills", len(globalSkills))
		installedSkills = m.mergeSkills(info.InstalledSkills, globalSkills)
		logger.Info("[Registry] RegisterNode: after merge, total InstalledSkills=%d", len(installedSkills))
	}

	// Also merge skill-provided capabilities into the node's capabilities.
	capabilities := info.Capabilities
	if len(installedSkills) > 0 {
		capabilities = m.mergeCapabilities(info.Capabilities, installedSkills)
	}

	state := &NodeState{
		Spec: NodeSpec{
			NodeID:          info.Id,
			NodeName:        info.Name,
			GRPCAddress:     info.Address,
			Capabilities:    capabilities,
			InstalledSkills: installedSkills,
			SystemInfo:      info.SystemInfo,
			Labels:          info.Labels,
			Version:         info.Version,
			Cordoned:        false,
		},
		Status: NodeStatus{
			Phase:         pb.NodeStatus_NODE_STATUS_ONLINE,
			Load:          load,
			LastHeartbeat: time.Now(),
			RegisteredAt:  time.Now(),
		},
	}

	m.nodes[info.Id] = state
	logger.Info("[Registry] registered node: id=%s name=%s addr=%s caps=%d skills=%d (enricher=%v)",
		info.Id, info.Name, info.Address, len(capabilities), len(installedSkills), m.enricher != nil)
	return nil
}

// mergeSkills merges Golem-reported skills with Hivemind-side global skills.
// Golem-reported skills take precedence by name.
func (m *InMemoryRegistry) mergeSkills(golemSkills, globalSkills []*pb.InstalledSkill) []*pb.InstalledSkill {
	seen := make(map[string]struct{})
	var merged []*pb.InstalledSkill

	// Golem-reported skills first (higher priority).
	for _, s := range golemSkills {
		seen[s.Name] = struct{}{}
		merged = append(merged, s)
	}

	// Add global skills that weren't already reported by Golem.
	for _, s := range globalSkills {
		if _, exists := seen[s.Name]; !exists {
			merged = append(merged, s)
		}
	}

	return merged
}

// mergeCapabilities merges existing capabilities with those declared by installed skills.
func (m *InMemoryRegistry) mergeCapabilities(existing []*pb.Capability, skills []*pb.InstalledSkill) []*pb.Capability {
	capSet := make(map[string]struct{})
	for _, c := range existing {
		capSet[c.Name] = struct{}{}
	}

	merged := make([]*pb.Capability, len(existing))
	copy(merged, existing)

	for _, sk := range skills {
		for _, capName := range sk.Capabilities {
			if _, exists := capSet[capName]; !exists {
				capSet[capName] = struct{}{}
				merged = append(merged, &pb.Capability{
					Name:        capName,
					Version:     "1.0",
					Description: fmt.Sprintf("Provided by skill %q", sk.Name),
				})
			}
		}
	}

	return merged
}

func (m *InMemoryRegistry) DeregisterNode(ctx context.Context, nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.nodes[nodeID]; !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	delete(m.nodes, nodeID)

	logger.Info("[Registry] deregistered node: %s", nodeID)
	return nil
}

func (m *InMemoryRegistry) UpdateHeartbeat(ctx context.Context, nodeID string, load *pb.NodeLoadInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	state.Status.LastHeartbeat = time.Now()
	state.Status.Load = load
	if load != nil {
		state.Status.RunningTasks = load.ActiveTasks
	}

	return nil
}

func (m *InMemoryRegistry) GetNode(nodeID string) (*NodeState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}
	return state, nil
}

func (m *InMemoryRegistry) ListNodes(filter *NodeFilter) ([]*NodeState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*NodeState
	for _, state := range m.nodes {
		if filter != nil && filter.StatusFilter != pb.NodeStatus_NODE_STATUS_UNSPECIFIED {
			if state.Status.Phase != filter.StatusFilter {
				continue
			}
		}
		result = append(result, state)
	}
	return result, nil
}

func (m *InMemoryRegistry) CordonNode(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	state.Spec.Cordoned = true
	state.Status.Phase = pb.NodeStatus_NODE_STATUS_CORDONED
	logger.Info("[Registry] cordoned node: %s", nodeID)
	return nil
}

func (m *InMemoryRegistry) UncordonNode(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	state.Spec.Cordoned = false
	state.Status.Phase = pb.NodeStatus_NODE_STATUS_ONLINE
	logger.Info("[Registry] uncordoned node: %s", nodeID)
	return nil
}

func (m *InMemoryRegistry) DrainNode(ctx context.Context, nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	state.Status.Phase = pb.NodeStatus_NODE_STATUS_DRAINING
	logger.Info("[Registry] draining node: %s", nodeID)
	return nil
}

func (m *InMemoryRegistry) FindCapableNodes(capabilities []string) ([]*NodeState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*NodeState
	for _, state := range m.nodes {
		// Skip offline, draining, or cordoned nodes.
		if state.Status.Phase != pb.NodeStatus_NODE_STATUS_ONLINE {
			continue
		}
		if state.Spec.Cordoned {
			continue
		}

		// Check if node has all required capabilities.
		if hasCapabilities(state.Spec.Capabilities, capabilities) {
			result = append(result, state)
		}
	}
	return result, nil
}

func (m *InMemoryRegistry) Start(ctx context.Context) error {
	hcCtx, hcCancel := context.WithCancel(ctx)
	m.cancel = hcCancel
	go m.healthCheckLoop(hcCtx)
	logger.Info("[Registry] started (heartbeat_timeout=%s, cleanup_interval=%s)",
		m.cfg.HeartbeatTimeout, m.cfg.CleanupInternal)
	return nil
}

func (m *InMemoryRegistry) Stop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	select {
	case <-m.stopped:
	case <-time.After(5 * time.Second):
	}

	logger.Info("[Registry] stopped")
	return nil
}

// hasCapabilities checks if a node's capabilities contain all required ones.
func hasCapabilities(nodeCaps []*pb.Capability, required []string) bool {
	if len(required) == 0 {
		return true
	}
	capSet := make(map[string]struct{}, len(nodeCaps))
	for _, c := range nodeCaps {
		capSet[c.Name] = struct{}{}
	}
	for _, req := range required {
		if _, ok := capSet[req]; !ok {
			return false
		}
	}
	return true
}
