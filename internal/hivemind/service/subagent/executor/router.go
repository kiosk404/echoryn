// Package executor provides the execution routing layer for SubAgent tasks.
// It routes execution requests to either local or remote (Golem) executors
// based on the configured execution strategy.
//
// Architecture:
//
//	ExecutionRouter
//	  ├── LocalExecutor  (wraps existing AgentRunner — current behavior)
//	  └── GolemExecutor  (NEW — dispatches to remote Golem nodes)
package executor

import (
	"context"
	"fmt"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/subagent/observer"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// ExecutionStrategy defines where a SubAgent should execute.
type ExecutionStrategy string

const (
	// ExecutionStrategyLocal executes the SubAgent in the local Hivemind process.
	ExecutionStrategyLocal ExecutionStrategy = "local"

	// ExecutionStrategyGolem dispatches the SubAgent to a remote Golem node.
	ExecutionStrategyGolem ExecutionStrategy = "golem"

	// ExecutionStrategyAuto lets the router decide based on resource availability.
	ExecutionStrategyAuto ExecutionStrategy = "auto"
)

// IsValid returns true if the strategy is a known value.
func (s ExecutionStrategy) IsValid() bool {
	switch s {
	case ExecutionStrategyLocal, ExecutionStrategyGolem, ExecutionStrategyAuto:
		return true
	}
	return false
}

// NodeAffinityPolicy defines node placement preferences for Golem execution.
// These map to the Golem Scheduler's ScheduleHints.Affinity mechanism.
type NodeAffinityPolicy string

const (
	// NodeAffinityNone: no placement preference.
	NodeAffinityNone NodeAffinityPolicy = "none"

	// NodeAffinityPreferColocate: prefer placing on the same node as team members.
	NodeAffinityPreferColocate NodeAffinityPolicy = "prefer_colocate"

	// NodeAffinityRequireColocate: require same-node placement.
	NodeAffinityRequireColocate NodeAffinityPolicy = "require_colocate"

	// NodeAffinityPreferSpread: prefer distributing across different nodes.
	NodeAffinityPreferSpread NodeAffinityPolicy = "prefer_spread"

	// NodeAffinityRequireSpread: require different-node placement.
	NodeAffinityRequireSpread NodeAffinityPolicy = "require_spread"
)

// ExecuteRequest extends the SubAgent execute request with routing metadata.
type ExecuteRequest struct {
	// AgentID is the agent to execute.
	AgentID string

	// SessionID is the session created for this SubAgent.
	SessionID string

	// Input is the task/prompt for the SubAgent.
	Input string

	// Strategy is the desired execution strategy.
	Strategy ExecutionStrategy

	// TeamID is the team this SubAgent belongs to (if any).
	TeamID string

	// NodeSelector is a label-based node selector for Golem execution.
	NodeSelector map[string]string

	// NodeAffinityPolicy controls node placement behavior.
	AffinityPolicy NodeAffinityPolicy
}

// Executor abstracts the actual execution of a SubAgent.
// The existing AgentExecutor interface is wrapped as a LocalExecutor.
type Executor interface {
	// Name returns the executor identifier.
	Name() string

	// Execute starts a SubAgent execution.
	Execute(ctx context.Context, req *ExecuteRequest) error
}

// ExecutionRouter routes SubAgent execution to the appropriate executor.
// It supports multiple registered executors and selects based on strategy.
//
// Design: Analogous to K8s kube-scheduler — receives a request, picks the best
// executor (node), and dispatches.
type ExecutionRouter interface {
	// Route selects the appropriate executor for the given request.
	Route(ctx context.Context, req *ExecuteRequest) (Executor, ExecutionStrategy, error)

	// RegisterExecutor registers an executor for a specific strategy.
	RegisterExecutor(strategy ExecutionStrategy, executor Executor)
}

// --- Default Router Implementation ---

// defaultRouter is the default implementation of ExecutionRouter.
type defaultRouter struct {
	mu        sync.RWMutex
	executors map[ExecutionStrategy]Executor
	emitter   *observer.Emitter
}

// NewRouter creates a new ExecutionRouter with no executors registered.
func NewRouter() ExecutionRouter {
	return &defaultRouter{
		executors: make(map[ExecutionStrategy]Executor),
		emitter:   observer.NewEmitter(nil), // no-op by default
	}
}

// NewRouterWithObserver creates an ExecutionRouter with an attached Observer.
func NewRouterWithObserver(obs observer.Observer) ExecutionRouter {
	return &defaultRouter{
		executors: make(map[ExecutionStrategy]Executor),
		emitter:   observer.NewEmitter(obs),
	}
}

// RegisterExecutor adds an executor for a given strategy.
func (r *defaultRouter) RegisterExecutor(strategy ExecutionStrategy, executor Executor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[strategy] = executor
	logger.Info("[ExecutionRouter] registered executor: strategy=%s, name=%s", strategy, executor.Name())
}

// Route selects the best executor based on the request's strategy.
//
// Routing logic:
//   - "local" → use local executor
//   - "golem" → use golem executor, fallback to local if unavailable
//   - "auto" → prefer golem if available, otherwise local
func (r *defaultRouter) Route(_ context.Context, req *ExecuteRequest) (Executor, ExecutionStrategy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	strategy := req.Strategy
	if strategy == "" {
		strategy = ExecutionStrategyLocal
	}

	var exec Executor
	var resolvedStrategy ExecutionStrategy
	var err error

	switch strategy {
	case ExecutionStrategyLocal:
		e, ok := r.executors[ExecutionStrategyLocal]
		if !ok {
			return nil, "", fmt.Errorf("local executor not registered")
		}
		exec, resolvedStrategy = e, ExecutionStrategyLocal

	case ExecutionStrategyGolem:
		e, ok := r.executors[ExecutionStrategyGolem]
		if ok {
			exec, resolvedStrategy = e, ExecutionStrategyGolem
		} else {
			// Fallback to local if Golem executor not available (scheduling phase failure).
			localExec, ok := r.executors[ExecutionStrategyLocal]
			if ok {
				logger.Warn("[ExecutionRouter] golem executor not available, falling back to local")
				r.emitter.Fallback("", req.SessionID, string(ExecutionStrategyGolem), "golem executor not available")
				exec, resolvedStrategy = localExec, ExecutionStrategyLocal
			} else {
				err = fmt.Errorf("no executor available (golem unavailable, local not registered)")
			}
		}

	case ExecutionStrategyAuto:
		// Prefer golem if available.
		if golemExec, ok := r.executors[ExecutionStrategyGolem]; ok {
			exec, resolvedStrategy = golemExec, ExecutionStrategyGolem
		} else if localExec, ok := r.executors[ExecutionStrategyLocal]; ok {
			exec, resolvedStrategy = localExec, ExecutionStrategyLocal
		} else {
			err = fmt.Errorf("no executor registered")
		}

	default:
		err = fmt.Errorf("unknown execution strategy: %s", strategy)
	}

	if err != nil {
		return nil, "", err
	}

	// Emit routing event.
	var location observer.ExecutionLocation
	switch resolvedStrategy {
	case ExecutionStrategyLocal:
		location = observer.LocationLocal
	case ExecutionStrategyGolem:
		location = observer.LocationGolem
	}
	r.emitter.Routed("", req.SessionID, location, string(resolvedStrategy), "", req.TeamID)

	return exec, resolvedStrategy, nil
}
