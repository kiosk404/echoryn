package memory_core

import (
	"sync"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory/memory-core/entity"
)

// SessionMemoryTracker tracks per-session state for the post-turn
// memory extraction feature. It maintains turn counts and token
// accumulation to implement dual-threshold triggering.
//
// Thread-safe: all mutations are guarded by a mutex.
// Lifecycle: created on first access for a session, never explicitly
// cleaned up (acceptable given bounded session count).
type SessionMemoryTracker struct {
	mu sync.Mutex

	// Turns since last extraction.
	turns int

	// Tokens accumulated since last extraction.
	tokens int

	// LastExtract records when the last extraction occurred.
	// Used for min-interval protection.
	LastExtract time.Time

	// totalExtracts tracks how many times extraction has fired.
	totalExtracts int
}

// NewSessionMemoryTracker creates a fresh tracker with LastExtract set to now
// so that immediate first extract requires threshold to actually be met.
func NewSessionMemoryTracker() *SessionMemoryTracker {
	return &SessionMemoryTracker{
		LastExtract: time.Now(),
	}
}

// RecordTurn records a completed turn's token usage and increments the turn counter.
func (t *SessionMemoryTracker) RecordTurn(tokensUsed int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.turns++
	t.tokens += tokensUsed
}

// TurnCount returns the current turn count since last reset.
func (t *SessionMemoryTracker) TurnCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.turns
}

// TokenCount returns the current token accumulation since last reset.
func (t *SessionMemoryTracker) TokenCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tokens
}

// TotalExtracts returns how many times extraction has been triggered.
func (t *SessionMemoryTracker) TotalExtracts() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.totalExtracts
}

// ShouldExtract checks if extraction should be triggered based on:
//  1. Turn threshold (turns >= cfg.TurnThreshold), OR
//  2. Token threshold (tokens >= cfg.TokenThreshold)
//
// Plus guard: minimum interval since last extraction.
func (t *SessionMemoryTracker) ShouldExtract(cfg entity.SessionMemoryConfig) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.turns < cfg.TurnThreshold && t.tokens < cfg.TokenThreshold {
		return false
	}

	if cfg.MinIntervalSec > 0 {
		if time.Since(t.LastExtract) < time.Duration(cfg.MinIntervalSec)*time.Second {
			return false
		}
	}

	return true
}

// Reset resets counters after a successful extraction.
func (t *SessionMemoryTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.turns = 0
	t.tokens = 0
	t.LastExtract = time.Now()
	t.totalExtracts++
}
