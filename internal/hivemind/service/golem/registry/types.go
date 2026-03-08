package registry

import (
	"context"
	"time"

	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
)

// SkillsEnricher provider Hivemind-side skills information to enrich Golem node registrations.
// When a Golem node registers without reporting its own skills (e.g. because it doesn't have a local
// skills directory), the Registry uses this enricher to populate the node's
// InstalledSkills from the Hivemind's globally-known skill catalog.
type SkillsEnricher interface {
	// GetGlobalSkills returns all skills known to the hivemind (from the skills plugin).
	// These will be merged into the node's InstalledSkills on registration.
	GetGlobalSkills() []*pb.InstalledSkill
}

// Registry manages all registered Golem nodes.
type Registry interface {
	// RegisterNode registers a new Golem node.
	RegisterNode(ctx context.Context, info *pb.NodeInfo, load *pb.NodeLoadInfo) error
	// DeregisterNode removes a Golem node from the registry.
	DeregisterNode(ctx context.Context, nodeID string) error
	// UpdateHeartbeat updates a node's heartbeat timestamp and load info.
	UpdateHeartbeat(ctx context.Context, nodeID string, load *pb.NodeLoadInfo) error

	// GetNode returns a single node's state.
	GetNode(nodeID string) (*NodeState, error)
	// ListNodes returns all nodes matching the filter.
	ListNodes(filter *NodeFilter) ([]*NodeState, error)

	// CordonNode marks a node as unschedulable.
	CordonNode(nodeID string) error
	// UncordonNode marks a node as schedulable again.
	UncordonNode(nodeID string) error
	// DrainNode sets a node to draining state.
	DrainNode(ctx context.Context, nodeID string) error

	// FindCapableNodes returns nodes that have the specified capabilities.
	FindCapableNodes(capabilities []string) ([]*NodeState, error)

	// SetSkillsEnricher sets the SkillsEnricher used to populate node skills on registration.
	SetSkillsEnricher(enricher SkillsEnricher)

	// Start starts the registry (health check loop, etc.).
	Start(ctx context.Context) error
	// Stop stops the registry.
	Stop(ctx context.Context) error
}

// NodeState represents the full state of a Golem node (Spec + Status, K8s style).
type NodeState struct {
	Spec   NodeSpec
	Status NodeStatus
}

// NodeSpec is the declared (semi-static) description of a node.
type NodeSpec struct {
	NodeID          string
	NodeName        string
	GRPCAddress     string
	Capabilities    []*pb.Capability
	InstalledSkills []*pb.InstalledSkill
	SystemInfo      *pb.SystemInfo
	Labels          map[string]string
	Version         string
	Cordoned        bool
}

// NodeStatus is the observed (dynamic) runtime state of a node.
type NodeStatus struct {
	Phase         pb.NodeStatus
	Load          *pb.NodeLoadInfo
	LastHeartbeat time.Time
	RegisteredAt  time.Time
	RunningTasks  int32
}

// NodeFilter for listing nodes.
type NodeFilter struct {
	StatusFilter pb.NodeStatus
	PageSize     int32
	PageToken    string
}
