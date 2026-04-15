package registry

import (
	"context"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
)

func (m *InMemoryRegistry) healthCheckLoop(ctx context.Context) {
	defer close(m.stopped)

	ticker := time.NewTicker(m.cfg.CleanupInternal)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkNodeHealth()
		}
	}
}

// checkNodeHealth iterates all nodes and marks stale ones as offline.
func (m *InMemoryRegistry) checkNodeHealth() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for nodeID, state := range m.nodes {
		// Skip already offline nodes.
		if state.Status.Phase == pb.NodeStatus_NODE_STATUS_OFFLINE {
			continue
		}
		// Check if heartbeat has timed out
		elapsed := now.Sub(state.Status.LastHeartbeat)
		if elapsed > m.cfg.HeartbeatTimeout {
			logger.Warn("[Registry] node %s heartbeat timeout (last=%s) ago, marking offline",
				nodeID, elapsed.Round(time.Second))
			state.Status.Phase = pb.NodeStatus_NODE_STATUS_OFFLINE
		}
	}
}
