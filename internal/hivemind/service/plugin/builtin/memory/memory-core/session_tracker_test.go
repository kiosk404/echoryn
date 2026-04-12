package memory_core

import (
	"sync"
	"testing"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/memory/memory-core/entity"
)

func TestNewSessionMemoryTracker(t *testing.T) {
	tracker := NewSessionMemoryTracker()
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}
	if tracker.TurnCount() != 0 {
		t.Fatalf("expected 0 turns, got %d", tracker.TurnCount())
	}
	if tracker.TokenCount() != 0 {
		t.Fatalf("expected 0 tokens, got %d", tracker.TokenCount())
	}
	if tracker.TotalExtracts() != 0 {
		t.Fatalf("expected 0 extracts, got %d", tracker.TotalExtracts())
	}
}

func TestRecordTurn_Accumulates(t *testing.T) {
	tracker := NewSessionMemoryTracker()
	tracker.RecordTurn(100)
	tracker.RecordTurn(200)
	tracker.RecordTurn(300)

	if tracker.TurnCount() != 3 {
		t.Fatalf("expected 3 turns, got %d", tracker.TurnCount())
	}
	if tracker.TokenCount() != 600 {
		t.Fatalf("expected 600 tokens, got %d", tracker.TokenCount())
	}
}

func TestShouldExtract_TurnThreshold(t *testing.T) {
	cfg := entity.SessionMemoryConfig{
		TurnThreshold:  3,
		TokenThreshold: 99999,
		MinIntervalSec: 0,
	}
	tracker := NewSessionMemoryTracker()

	tracker.RecordTurn(100)
	tracker.RecordTurn(100)
	if tracker.ShouldExtract(cfg) {
		t.Fatal("expected false below turn threshold")
	}

	tracker.RecordTurn(100)
	if !tracker.ShouldExtract(cfg) {
		t.Fatal("expected true at turn threshold")
	}
}

func TestShouldExtract_TokenThreshold(t *testing.T) {
	cfg := entity.SessionMemoryConfig{
		TurnThreshold:  99999,
		TokenThreshold: 5000,
		MinIntervalSec: 0,
	}
	tracker := NewSessionMemoryTracker()

	tracker.RecordTurn(3000)
	if tracker.ShouldExtract(cfg) {
		t.Fatal("expected false below token threshold")
	}

	tracker.RecordTurn(3000)
	if !tracker.ShouldExtract(cfg) {
		t.Fatal("expected true at token threshold")
	}
}

func TestShouldExtract_BothBelowThreshold(t *testing.T) {
	cfg := entity.DefaultSessionMemoryConfig()
	tracker := NewSessionMemoryTracker()

	tracker.RecordTurn(100)
	tracker.RecordTurn(100)
	if tracker.ShouldExtract(cfg) {
		t.Fatal("expected false when both below threshold")
	}
}

func TestShouldExtract_MinIntervalProtection(t *testing.T) {
	cfg := entity.SessionMemoryConfig{
		TurnThreshold:  1,
		TokenThreshold: 0,
		MinIntervalSec: 10,
	}
	tracker := NewSessionMemoryTracker()

	// First attempt: LastExtract was just set to now, so interval blocks even first try
	tracker.RecordTurn(100)
	if tracker.ShouldExtract(cfg) {
		t.Fatal("expected false on first trigger (interval blocks)")
	}

	// Simulate enough time passing by manually setting LastExtract far in past
	tracker.mu.Lock()
	tracker.LastExtract = time.Now().Add(-20 * time.Second)
	tracker.mu.Unlock()

	if !tracker.ShouldExtract(cfg) {
		t.Fatal("expected true after interval elapsed")
	}
	tracker.Reset()

	// Immediate second trigger should be blocked by interval
	tracker.RecordTurn(100)
	if tracker.ShouldExtract(cfg) {
		t.Fatal("expected false due to min interval protection")
	}
}

func TestReset_ClearsCounters(t *testing.T) {
	cfg := entity.DefaultSessionMemoryConfig()
	cfg.MinIntervalSec = 0 // disable for this test
	tracker := NewSessionMemoryTracker()

	for i := 0; i < 20; i++ {
		tracker.RecordTurn(500)
	}
	if !tracker.ShouldExtract(cfg) {
		t.Fatal("expected true before reset")
	}

	// Simulate the full lifecycle: ShouldExtract → do work → Reset
	tracker.Reset()
	if tracker.TotalExtracts() != 1 {
		t.Fatalf("expected 1 extract after first reset, got %d", tracker.TotalExtracts())
	}

	if tracker.TurnCount() != 0 {
		t.Fatalf("expected 0 turns after reset, got %d", tracker.TurnCount())
	}
	if tracker.TokenCount() != 0 {
		t.Fatalf("expected 0 tokens after reset, got %d", tracker.TokenCount())
	}
	if tracker.ShouldExtract(cfg) {
		t.Fatal("expected false after reset (below threshold)")
	}
}

func TestConcurrentRecordTurn(t *testing.T) {
	cfg := entity.SessionMemoryConfig{
		TurnThreshold:  100,
		TokenThreshold: 50000,
		MinIntervalSec: 0,
	}
	tracker := NewSessionMemoryTracker()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.RecordTurn(500)
		}()
	}
	wg.Wait()

	if tracker.TurnCount() != 100 {
		t.Fatalf("expected 100 turns, got %d", tracker.TurnCount())
	}
	if tracker.TokenCount() != 50000 {
		t.Fatalf("expected 50000 tokens, got %d", tracker.TokenCount())
	}
	if !tracker.ShouldExtract(cfg) {
		t.Fatal("expected true after concurrent recording")
	}
}

// TestConcurrentRecordTurn_Race uses -race to detect data races.
// Run with: go test -race ./... -run TestConcurrentRecordTurn
func TestConcurrentRecordTurn_Race(t *testing.T) {
	cfg := entity.SessionMemoryConfig{
		TurnThreshold:  50,
		TokenThreshold: 25000,
		MinIntervalSec: 0,
	}
	tracker := NewSessionMemoryTracker()

	var wg sync.WaitGroup
	// Mix of RecordTurn and ShouldExtract calls concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tracker.RecordTurn(n)
			_ = tracker.ShouldExtract(cfg)
			_ = tracker.TurnCount()
			_ = tracker.TokenCount()
		}(i)
	}
	wg.Wait()
	// If we get here without -race detecting issues, the test passes
}
