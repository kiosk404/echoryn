// Package toolloop implements tool call loop detection aligned with OpenClaw's
// tool-loop-detection.ts. It detects and blocks infinite tool call cycles
// to replace the hard MaxStep limit with intelligent circuit-breaking.
//
// Detection is performed by four detectors run in priority order:
//
//  1. global_circuit_breaker — any tool repeated ≥ GlobalCircuitBreakerThreshold
//     times without progress → critical block
//  2. known_poll_no_progress — known polling tools (process poll/log,
//     command_status) repeated without progress → warning at WarningThreshold,
//     critical at CriticalThreshold
//  3. ping_pong — two tools alternating (A→B→A→B) → warning/critical
//  4. generic_repeat — same tool+args repeated → warning only (never blocks
//     on its own, the global circuit breaker handles that)
package toolloop

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// Level indicates the severity of a detected loop.
type Level string

const (
	LevelNone     Level = ""
	LevelWarning  Level = "warning"
	LevelCritical Level = "critical"
)

// DetectorKind identifies which detector triggered the result.
type DetectorKind string

const (
	DetectorGlobalCircuitBreaker DetectorKind = "global_circuit_breaker"
	DetectorKnownPollNoProgress  DetectorKind = "known_poll_no_progress"
	DetectorPingPong             DetectorKind = "ping_pong"
	DetectorGenericRepeat        DetectorKind = "generic_repeat"
)

// Result is the outcome of a loop detection check.
type Result struct {
	Stuck    bool
	Level    Level
	Detector DetectorKind
	Message  string
}

// Config controls the loop detection thresholds.
// Aligned with OpenClaw's ToolLoopDetectionConfig.
type Config struct {
	Enabled                       bool
	HistorySize                   int
	WarningThreshold              int
	CriticalThreshold             int
	GlobalCircuitBreakerThreshold int
}

// DefaultConfig returns the default config aligned with OpenClaw.
func DefaultConfig() Config {
	return Config{
		Enabled:                       true,
		HistorySize:                   30,
		WarningThreshold:              10,
		CriticalThreshold:             20,
		GlobalCircuitBreakerThreshold: 30,
	}
}

// entry is a single recorded tool call.
type entry struct {
	ToolName    string
	Hash        string // sha256(toolName + args)
	HasProgress bool   // whether the outcome indicated progress
}

// Detector tracks tool call history for a single session/run and detects loops.
// Thread-safe.
type Detector struct {
	mu      sync.Mutex
	cfg     Config
	history []entry
}

// NewDetector creates a new Detector with the given config.
func NewDetector(cfg Config) *Detector {
	return &Detector{
		cfg:     cfg,
		history: make([]entry, 0, cfg.HistorySize),
	}
}

// knownPollTools lists tool names considered "polling" tools.
// Aligned with OpenClaw's KNOWN_POLL_TOOL_NAMES.
var knownPollTools = map[string]bool{
	"command_status":      true,
	"process_poll":        true,
	"process_log":         true,
	"wait_for_completion": true,
}

// hashToolCall creates a unique signature for a tool call.
func hashToolCall(toolName, args string) string {
	h := sha256.Sum256([]byte(toolName + ":" + args))
	return hex.EncodeToString(h[:8]) // 16 hex chars is plenty
}

// Check detects whether the upcoming tool call (toolName + args) would form a loop.
// This should be called BEFORE executing the tool.
func (d *Detector) Check(toolName, args string) Result {
	if !d.cfg.Enabled {
		return Result{}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	hash := hashToolCall(toolName, args)

	// --- Detector 1: Global circuit breaker ---
	noProgressCount := 0
	for _, e := range d.history {
		if !e.HasProgress {
			noProgressCount++
		}
	}
	if noProgressCount >= d.cfg.GlobalCircuitBreakerThreshold {
		return Result{
			Stuck:    true,
			Level:    LevelCritical,
			Detector: DetectorGlobalCircuitBreaker,
			Message: fmt.Sprintf(
				"global circuit breaker: %d tool calls without meaningful progress, stopping",
				noProgressCount),
		}
	}

	// Count exact-match repetitions of this tool+args in history.
	exactRepeatCount := 0
	exactNoProgressCount := 0
	for _, e := range d.history {
		if e.Hash == hash {
			exactRepeatCount++
			if !e.HasProgress {
				exactNoProgressCount++
			}
		}
	}

	// --- Detector 2: Known poll tool with no progress ---
	if isKnownPollTool(toolName) {
		if exactNoProgressCount >= d.cfg.CriticalThreshold {
			return Result{
				Stuck:    true,
				Level:    LevelCritical,
				Detector: DetectorKnownPollNoProgress,
				Message: fmt.Sprintf(
					"known polling tool %q repeated %d times without progress, stopping",
					toolName, exactNoProgressCount),
			}
		}
		if exactNoProgressCount >= d.cfg.WarningThreshold {
			return Result{
				Stuck:    true,
				Level:    LevelWarning,
				Detector: DetectorKnownPollNoProgress,
				Message: fmt.Sprintf(
					"known polling tool %q repeated %d times without progress (warning)",
					toolName, exactNoProgressCount),
			}
		}
	}

	// --- Detector 3: Ping-pong detection ---
	if pingPongResult := d.detectPingPong(toolName, hash); pingPongResult.Stuck {
		return pingPongResult
	}

	// --- Detector 4: Generic repeat ---
	if exactRepeatCount >= d.cfg.WarningThreshold && !isKnownPollTool(toolName) {
		return Result{
			Stuck:    true,
			Level:    LevelWarning,
			Detector: DetectorGenericRepeat,
			Message: fmt.Sprintf(
				"tool %q called with same arguments %d times (warning, not blocking)",
				toolName, exactRepeatCount),
		}
	}

	return Result{}
}

// detectPingPong checks for alternating A→B→A→B patterns.
func (d *Detector) detectPingPong(toolName, hash string) Result {
	if len(d.history) < 3 {
		return Result{}
	}

	// Find the "other" tool in potential ping-pong.
	last := d.history[len(d.history)-1]
	if last.ToolName == toolName {
		return Result{} // Same tool twice in a row — not ping-pong
	}

	// Count alternating pattern: current → last → current → last ...
	otherHash := last.Hash
	alternateCount := 0
	alternateNoProgress := 0
	expectHash := hash // We expect: ...hash, otherHash, hash, otherHash...

	for i := len(d.history) - 1; i >= 0; i-- {
		e := d.history[i]
		if i == len(d.history)-1 {
			expectHash = otherHash
		}
		if e.Hash != expectHash {
			break
		}
		alternateCount++
		if !e.HasProgress {
			alternateNoProgress++
		}
		// Toggle expected hash.
		if expectHash == hash {
			expectHash = otherHash
		} else {
			expectHash = hash
		}
	}

	totalPingPong := alternateCount + 1 // +1 for the upcoming call

	if totalPingPong >= d.cfg.CriticalThreshold && alternateNoProgress >= d.cfg.WarningThreshold {
		return Result{
			Stuck:    true,
			Level:    LevelCritical,
			Detector: DetectorPingPong,
			Message: fmt.Sprintf(
				"ping-pong loop detected: %q ↔ %q alternating %d times without progress, stopping",
				toolName, last.ToolName, totalPingPong),
		}
	}

	if totalPingPong >= d.cfg.WarningThreshold {
		return Result{
			Stuck:    true,
			Level:    LevelWarning,
			Detector: DetectorPingPong,
			Message: fmt.Sprintf(
				"ping-pong pattern detected: %q ↔ %q alternating %d times (warning)",
				toolName, last.ToolName, totalPingPong),
		}
	}

	return Result{}
}

// Record records a tool call in the history. Should be called AFTER Check.
func (d *Detector) Record(toolName, args string) {
	if !d.cfg.Enabled {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	hash := hashToolCall(toolName, args)
	d.history = append(d.history, entry{
		ToolName:    toolName,
		Hash:        hash,
		HasProgress: false, // Default; updated by RecordOutcome
	})

	// Sliding window: trim to HistorySize.
	if len(d.history) > d.cfg.HistorySize {
		d.history = d.history[len(d.history)-d.cfg.HistorySize:]
	}
}

// RecordOutcome updates the last recorded tool call's progress status.
// hasProgress=true means the tool returned meaningfully different output.
//
// For simplicity, we consider any non-error tool result as "progress".
// A more sophisticated implementation could compare output hashes.
func (d *Detector) RecordOutcome(hasProgress bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.history) > 0 {
		d.history[len(d.history)-1].HasProgress = hasProgress
	}
}

// Stats returns debugging statistics about the current history.
func (d *Detector) Stats() map[string]int {
	d.mu.Lock()
	defer d.mu.Unlock()

	stats := map[string]int{
		"total_calls":  len(d.history),
		"no_progress":  0,
		"unique_tools": 0,
	}

	toolSet := make(map[string]bool)
	for _, e := range d.history {
		if !e.HasProgress {
			stats["no_progress"]++
		}
		toolSet[e.ToolName] = true
	}
	stats["unique_tools"] = len(toolSet)

	return stats
}

func isKnownPollTool(name string) bool {
	if knownPollTools[name] {
		return true
	}
	// Also match by prefix/suffix patterns (aligned with OpenClaw's broader matching).
	lower := strings.ToLower(name)
	return strings.Contains(lower, "_status") ||
		strings.Contains(lower, "_poll") ||
		strings.HasSuffix(lower, "_wait")
}
