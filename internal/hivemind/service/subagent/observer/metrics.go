package observer

import (
	"math"
	"sort"
	"sync"
	"time"
)

// MetricsAggregator collects and aggregates SubAgent execution metrics.
//
// Design:
//   - Thread-safe: all state protected by mutex
//   - Rolling window: timing samples are bounded by maxSamples to prevent
//     unbounded memory growth
//   - Per-agent and per-team breakdowns: track fine-grained metrics for
//     each agent and team independently
//
// Analogous to Golem's StatsCollector (scheduler/monitor.go), but focused
// on the SubAgent execution domain rather than Golem node scheduling.
type MetricsAggregator struct {
	mu sync.Mutex

	// Global counters
	totalSpawned   int64
	totalCompleted int64
	totalFailed    int64
	totalCancelled int64
	totalTimeout   int64
	totalFallback  int64

	// Location counters
	localExecutions int64
	golemExecutions int64

	// Active state tracking
	runningTasks map[string]time.Time // recordID → startedAt
	pendingTasks map[string]time.Time // recordID → spawnedAt

	// Timing samples (rolling window)
	executionSamples []time.Duration
	scheduleSamples  []time.Duration
	maxSamples       int

	// Per-agent metrics
	agentStats map[string]*agentStatsEntry

	// Per-team metrics
	teamStats map[string]*teamStatsEntry

	// Window start time
	windowStart time.Time
}

type agentStatsEntry struct {
	spawned      int64
	completed    int64
	failed       int64
	execSamples  []time.Duration
	maxSampleCnt int
}

type teamStatsEntry struct {
	totalTasks int64
	completed  int64
	failed     int64
	execSample []time.Duration
	members    map[string]bool // agentID set
	nodes      map[string]bool // nodeID set
}

// NewMetricsAggregator creates a new aggregator with the given sample window size.
func NewMetricsAggregator(maxSamples int) *MetricsAggregator {
	if maxSamples <= 0 {
		maxSamples = 500
	}
	return &MetricsAggregator{
		runningTasks: make(map[string]time.Time),
		pendingTasks: make(map[string]time.Time),
		agentStats:   make(map[string]*agentStatsEntry),
		teamStats:    make(map[string]*teamStatsEntry),
		maxSamples:   maxSamples,
		windowStart:  time.Now(),
	}
}

// Record processes an event and updates the aggregated metrics.
func (m *MetricsAggregator) Record(event Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch event.Kind {
	case EventSpawned:
		m.totalSpawned++
		m.pendingTasks[event.RecordID] = event.Timestamp
		m.getOrCreateAgent(event.AgentID).spawned++
		if event.TeamID != "" {
			ts := m.getOrCreateTeam(event.TeamID)
			ts.totalTasks++
			if event.AgentID != "" {
				ts.members[event.AgentID] = true
			}
		}

	case EventScheduled:
		// Compute scheduling latency (time from spawn to scheduled).
		if spawnedAt, ok := m.pendingTasks[event.RecordID]; ok {
			latency := event.Timestamp.Sub(spawnedAt)
			m.scheduleSamples = appendBounded(m.scheduleSamples, latency, m.maxSamples)
		}

	case EventRunning:
		// Move from pending to running.
		delete(m.pendingTasks, event.RecordID)
		m.runningTasks[event.RecordID] = event.Timestamp

	case EventCompleted:
		m.totalCompleted++
		m.recordTerminal(event)
		m.getOrCreateAgent(event.AgentID).completed++
		if event.TeamID != "" {
			m.getOrCreateTeam(event.TeamID).completed++
		}

	case EventFailed:
		m.totalFailed++
		m.recordTerminal(event)
		m.getOrCreateAgent(event.AgentID).failed++
		if event.TeamID != "" {
			m.getOrCreateTeam(event.TeamID).failed++
		}

	case EventCancelled:
		m.totalCancelled++
		m.recordTerminal(event)

	case EventTimeout:
		m.totalTimeout++
		m.recordTerminal(event)

	case EventFallback:
		m.totalFallback++

	case EventRouted:
		switch event.Location {
		case LocationLocal:
			m.localExecutions++
		case LocationGolem:
			m.golemExecutions++
			if event.TeamID != "" && event.NodeID != "" {
				m.getOrCreateTeam(event.TeamID).nodes[event.NodeID] = true
			}
		}
	}
}

// recordTerminal handles common logic for terminal events.
func (m *MetricsAggregator) recordTerminal(event Event) {
	// Remove from active tracking.
	delete(m.pendingTasks, event.RecordID)

	if startedAt, ok := m.runningTasks[event.RecordID]; ok {
		execTime := event.Timestamp.Sub(startedAt)
		m.executionSamples = appendBounded(m.executionSamples, execTime, m.maxSamples)

		// Per-agent exec time.
		if event.AgentID != "" {
			as := m.getOrCreateAgent(event.AgentID)
			as.execSamples = appendBounded(as.execSamples, execTime, as.maxSampleCnt)
		}

		// Per-team exec time.
		if event.TeamID != "" {
			ts := m.getOrCreateTeam(event.TeamID)
			ts.execSample = appendBounded(ts.execSample, execTime, m.maxSamples)
		}

		delete(m.runningTasks, event.RecordID)
	} else if event.Duration > 0 {
		// Fallback: use the duration field directly if we missed the running event.
		m.executionSamples = appendBounded(m.executionSamples, event.Duration, m.maxSamples)
	}
}

// Snapshot returns a copy of the current metrics.
func (m *MetricsAggregator) Snapshot() ExecutionMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := ExecutionMetrics{
		TotalSpawned:       m.totalSpawned,
		TotalCompleted:     m.totalCompleted,
		TotalFailed:        m.totalFailed,
		TotalCancelled:     m.totalCancelled,
		TotalTimeout:       m.totalTimeout,
		TotalFallback:      m.totalFallback,
		LocalExecutions:    m.localExecutions,
		GolemExecutions:    m.golemExecutions,
		CurrentRunning:     int64(len(m.runningTasks)),
		CurrentPending:     int64(len(m.pendingTasks)),
		AvgExecutionTime:   averageDuration(m.executionSamples),
		AvgScheduleLatency: averageDuration(m.scheduleSamples),
		MaxExecutionTime:   maxDuration(m.executionSamples),
		P95ExecutionTime:   percentileDuration(m.executionSamples, 0.95),
		WindowStart:        m.windowStart,
		CollectedAt:        time.Now(),
	}

	// Per-agent metrics.
	if len(m.agentStats) > 0 {
		metrics.AgentMetrics = make(map[string]*AgentExecutionMetrics, len(m.agentStats))
		for agentID, as := range m.agentStats {
			total := as.completed + as.failed
			var successRate float64
			if total > 0 {
				successRate = float64(as.completed) / float64(total)
			}
			metrics.AgentMetrics[agentID] = &AgentExecutionMetrics{
				AgentID:        agentID,
				TotalSpawned:   as.spawned,
				TotalCompleted: as.completed,
				TotalFailed:    as.failed,
				AvgExecTime:    averageDuration(as.execSamples),
				SuccessRate:    math.Round(successRate*10000) / 10000, // 4 decimal places
			}
		}
	}

	// Per-team metrics.
	if len(m.teamStats) > 0 {
		metrics.TeamMetrics = make(map[string]*TeamExecutionMetrics, len(m.teamStats))
		for teamID, ts := range m.teamStats {
			metrics.TeamMetrics[teamID] = &TeamExecutionMetrics{
				TeamID:          teamID,
				TotalTasks:      ts.totalTasks,
				CompletedTasks:  ts.completed,
				FailedTasks:     ts.failed,
				AvgExecTime:     averageDuration(ts.execSample),
				MemberCount:     len(ts.members),
				ActiveNodeCount: len(ts.nodes),
			}
		}
	}

	return metrics
}

// Reset clears all metrics and starts a new window.
func (m *MetricsAggregator) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalSpawned = 0
	m.totalCompleted = 0
	m.totalFailed = 0
	m.totalCancelled = 0
	m.totalTimeout = 0
	m.totalFallback = 0
	m.localExecutions = 0
	m.golemExecutions = 0
	m.runningTasks = make(map[string]time.Time)
	m.pendingTasks = make(map[string]time.Time)
	m.executionSamples = nil
	m.scheduleSamples = nil
	m.agentStats = make(map[string]*agentStatsEntry)
	m.teamStats = make(map[string]*teamStatsEntry)
	m.windowStart = time.Now()
}

// --- Internal helpers ---

func (m *MetricsAggregator) getOrCreateAgent(agentID string) *agentStatsEntry {
	if agentID == "" {
		agentID = "unknown"
	}
	as, ok := m.agentStats[agentID]
	if !ok {
		as = &agentStatsEntry{maxSampleCnt: m.maxSamples}
		m.agentStats[agentID] = as
	}
	return as
}

func (m *MetricsAggregator) getOrCreateTeam(teamID string) *teamStatsEntry {
	ts, ok := m.teamStats[teamID]
	if !ok {
		ts = &teamStatsEntry{
			members: make(map[string]bool),
			nodes:   make(map[string]bool),
		}
		m.teamStats[teamID] = ts
	}
	return ts
}

// appendBounded appends a sample to a slice, maintaining a max capacity.
func appendBounded(samples []time.Duration, d time.Duration, maxCap int) []time.Duration {
	samples = append(samples, d)
	if len(samples) > maxCap {
		// Trim oldest samples.
		samples = samples[len(samples)-maxCap:]
	}
	return samples
}

// averageDuration computes the average of a duration slice.
func averageDuration(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	var total time.Duration
	for _, s := range samples {
		total += s
	}
	return total / time.Duration(len(samples))
}

// maxDuration returns the maximum value in a duration slice.
func maxDuration(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	max := samples[0]
	for _, s := range samples[1:] {
		if s > max {
			max = s
		}
	}
	return max
}

// percentileDuration computes the given percentile of a duration slice.
func percentileDuration(samples []time.Duration, p float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
