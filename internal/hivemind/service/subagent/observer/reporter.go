package observer

import (
	"fmt"
	"time"
)

// Reporter generates execution reports and evaluates health status
// based on aggregated metrics.
//
// Design:
//   - Stateless evaluation: each report is generated from the current
//     metrics snapshot, no internal state is carried between reports.
//   - Threshold-based alerts: configurable thresholds for failure rate,
//     timeout rate, and execution time anomalies.
//   - Health classification: healthy/degraded/critical based on alert severity.
type Reporter struct {
	metrics *MetricsAggregator
	config  Config
}

// NewReporter creates a Reporter backed by the given MetricsAggregator.
func NewReporter(metrics *MetricsAggregator, config Config) *Reporter {
	return &Reporter{
		metrics: metrics,
		config:  config,
	}
}

// Generate creates an ExecutionReport for the given time window.
// The window parameter controls which recent events are included.
func (r *Reporter) Generate(window time.Duration, recentEvents []Event) *ExecutionReport {
	snap := r.metrics.Snapshot()

	// Filter recent events by window.
	cutoff := time.Now().Add(-window)
	var windowEvents []Event
	for _, e := range recentEvents {
		if e.Timestamp.After(cutoff) {
			windowEvents = append(windowEvents, e)
		}
	}

	// Evaluate health and alerts.
	alerts := r.evaluateAlerts(snap)
	healthStatus, healthMsg := r.classifyHealth(alerts, snap)

	return &ExecutionReport{
		Window:        window,
		Metrics:       snap,
		HealthStatus:  healthStatus,
		HealthMessage: healthMsg,
		RecentEvents:  windowEvents,
		Alerts:        alerts,
		GeneratedAt:   time.Now(),
	}
}

// evaluateAlerts checks the current metrics against configured thresholds
// and returns any active alerts.
func (r *Reporter) evaluateAlerts(metrics ExecutionMetrics) []Alert {
	var alerts []Alert
	now := time.Now()

	// --- Failure rate alert ---
	totalTerminal := metrics.TotalCompleted + metrics.TotalFailed
	if totalTerminal > 0 {
		failureRate := float64(metrics.TotalFailed) / float64(totalTerminal)

		if failureRate >= r.config.CriticalFailureRateThreshold {
			alerts = append(alerts, Alert{
				Severity:   "critical",
				Type:       "high_failure_rate",
				Message:    fmt.Sprintf("Critical failure rate: %.1f%% (%d/%d tasks failed)", failureRate*100, metrics.TotalFailed, totalTerminal),
				DetectedAt: now,
				Context: map[string]string{
					"failure_rate":    fmt.Sprintf("%.4f", failureRate),
					"total_failed":    fmt.Sprintf("%d", metrics.TotalFailed),
					"total_completed": fmt.Sprintf("%d", metrics.TotalCompleted),
				},
			})
		} else if failureRate >= r.config.FailureRateThreshold {
			alerts = append(alerts, Alert{
				Severity:   "warning",
				Type:       "elevated_failure_rate",
				Message:    fmt.Sprintf("Elevated failure rate: %.1f%% (%d/%d tasks failed)", failureRate*100, metrics.TotalFailed, totalTerminal),
				DetectedAt: now,
				Context: map[string]string{
					"failure_rate": fmt.Sprintf("%.4f", failureRate),
					"total_failed": fmt.Sprintf("%d", metrics.TotalFailed),
				},
			})
		}
	}

	// --- Timeout rate alert ---
	if metrics.TotalSpawned > 0 {
		timeoutRate := float64(metrics.TotalTimeout) / float64(metrics.TotalSpawned)
		if timeoutRate >= r.config.TimeoutRateThreshold {
			alerts = append(alerts, Alert{
				Severity:   "warning",
				Type:       "high_timeout_rate",
				Message:    fmt.Sprintf("High timeout rate: %.1f%% (%d/%d tasks timed out)", timeoutRate*100, metrics.TotalTimeout, metrics.TotalSpawned),
				DetectedAt: now,
				Context: map[string]string{
					"timeout_rate":  fmt.Sprintf("%.4f", timeoutRate),
					"total_timeout": fmt.Sprintf("%d", metrics.TotalTimeout),
				},
			})
		}
	}

	// --- Pending tasks buildup alert ---
	if metrics.CurrentPending > 10 {
		alerts = append(alerts, Alert{
			Severity:   "warning",
			Type:       "pending_buildup",
			Message:    fmt.Sprintf("Pending task buildup: %d tasks waiting for execution", metrics.CurrentPending),
			DetectedAt: now,
			Context: map[string]string{
				"current_pending": fmt.Sprintf("%d", metrics.CurrentPending),
				"current_running": fmt.Sprintf("%d", metrics.CurrentRunning),
			},
		})
	}

	// --- Fallback rate alert ---
	totalRouted := metrics.LocalExecutions + metrics.GolemExecutions
	if totalRouted > 0 && metrics.TotalFallback > 0 {
		fallbackRate := float64(metrics.TotalFallback) / float64(totalRouted)
		if fallbackRate >= 0.5 {
			alerts = append(alerts, Alert{
				Severity:   "warning",
				Type:       "high_fallback_rate",
				Message:    fmt.Sprintf("High fallback rate: %.1f%% (%d fallbacks)", fallbackRate*100, metrics.TotalFallback),
				DetectedAt: now,
				Context: map[string]string{
					"fallback_rate":  fmt.Sprintf("%.4f", fallbackRate),
					"total_fallback": fmt.Sprintf("%d", metrics.TotalFallback),
				},
			})
		}
	}

	// --- Per-agent failure alerts ---
	for agentID, am := range metrics.AgentMetrics {
		if am.SuccessRate < (1.0-r.config.FailureRateThreshold) && (am.TotalCompleted+am.TotalFailed) >= 3 {
			alerts = append(alerts, Alert{
				Severity:   "warning",
				Type:       "agent_low_success_rate",
				Message:    fmt.Sprintf("Agent %s has low success rate: %.1f%%", agentID, am.SuccessRate*100),
				DetectedAt: now,
				Context: map[string]string{
					"agent_id":     agentID,
					"success_rate": fmt.Sprintf("%.4f", am.SuccessRate),
					"completed":    fmt.Sprintf("%d", am.TotalCompleted),
					"failed":       fmt.Sprintf("%d", am.TotalFailed),
				},
			})
		}
	}

	return alerts
}

// classifyHealth determines the overall health status based on active alerts.
func (r *Reporter) classifyHealth(alerts []Alert, metrics ExecutionMetrics) (HealthStatus, string) {
	if len(alerts) == 0 {
		if metrics.TotalSpawned == 0 {
			return HealthStatusHealthy, "No SubAgent activity recorded"
		}
		return HealthStatusHealthy, fmt.Sprintf("All systems nominal: %d spawned, %d completed, %d running",
			metrics.TotalSpawned, metrics.TotalCompleted, metrics.CurrentRunning)
	}

	// Check for critical alerts.
	for _, a := range alerts {
		if a.Severity == "critical" {
			return HealthStatusCritical, a.Message
		}
	}

	// Multiple warnings → degraded.
	if len(alerts) >= 2 {
		return HealthStatusDegraded, fmt.Sprintf("%d active warnings detected", len(alerts))
	}

	// Single warning → still degraded but with specific message.
	return HealthStatusDegraded, alerts[0].Message
}
