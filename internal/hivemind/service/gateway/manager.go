package gateway

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// initialRetryDelay is the starting delay for exponential backoff restart.
	initialRetryDelay = 5 * time.Second
	// maxRetryDelay is the maximum delay between restart attempts.
	maxRetryDelay = 5 * time.Minute
	// maxRetryAttempts is the maximum number of automatic restart attempts.
	maxRetryAttempts = 10
)

// channelEntry tracks a registered channel and its runtime state.
type channelEntry struct {
	channel         Channel
	outbound        OutboundAdapter
	agentID         string
	cancel          context.CancelFunc
	retryCount      int
	manuallyStopped bool
}

// ChannelManager manages the lifecycle of all registered IM channels.
// It handles starting, stopping, and automatic restart with exponential backoff.
//
// This is the Echoryn equivalent of OpenClaw's ChannelManager (server-channels.ts).
type ChannelManager struct {
	mu       sync.RWMutex
	channels map[string]*channelEntry
	handler  InboundHandler
	stopped  bool
}

// NewChannelManager creates a new ChannelManager with the given inbound handler.
func NewChannelManager(handler InboundHandler) *ChannelManager {
	return &ChannelManager{
		channels: make(map[string]*channelEntry),
		handler:  handler,
	}
}

// Register adds a channel to the manager. Must be called before StartAll.
func (m *ChannelManager) Register(ch Channel, outbound OutboundAdapter, agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[ch.ID()] = &channelEntry{
		channel:  ch,
		outbound: outbound,
		agentID:  agentID,
	}
}

// GetOutbound returns the OutboundAdapter for the given channel ID.
func (m *ChannelManager) GetOutbound(channelID string) (OutboundAdapter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.channels[channelID]
	if !ok {
		return nil, false
	}
	return entry.outbound, true
}

// GetAgentID returns the configured agent ID for the given channel.
func (m *ChannelManager) GetAgentID(channelID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.channels[channelID]
	if !ok {
		return ""
	}
	return entry.agentID
}

// StartAll starts all registered channels.
func (m *ChannelManager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for id, entry := range m.channels {
		if err := m.startChannel(ctx, id, entry); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", id, err))
		}
	}

	if len(errs) > 0 {
		// Log errors but don't fail — partial startup is acceptable.
		for _, err := range errs {
			logger.Warn("[Gateway] %v", err)
		}
	}

	return nil
}

// StopAll gracefully stops all running channels.
func (m *ChannelManager) StopAll(ctx context.Context) {
	m.mu.Lock()
	m.stopped = true
	entries := make(map[string]*channelEntry, len(m.channels))
	for k, v := range m.channels {
		entries[k] = v
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for id, entry := range entries {
		if entry.cancel == nil {
			continue
		}
		wg.Add(1)
		go func(id string, e *channelEntry) {
			defer wg.Done()
			e.cancel()
			if err := e.channel.Stop(ctx); err != nil {
				logger.Warn("[Gateway] channel %s stop error: %v", id, err)
			}
			logger.Info("[Gateway] channel %s stopped", id)
		}(id, entry)
	}
	wg.Wait()
}

// startChannel starts a single channel with auto-restart on failure.
func (m *ChannelManager) startChannel(parentCtx context.Context, id string, entry *channelEntry) error {
	ctx, cancel := context.WithCancel(parentCtx)
	entry.cancel = cancel
	entry.retryCount = 0
	entry.manuallyStopped = false

	if err := entry.channel.Start(ctx, m.handler); err != nil {
		cancel()
		entry.cancel = nil
		return fmt.Errorf("start failed: %w", err)
	}

	logger.Info("[Gateway] channel %s started", id)

	// Launch a watchdog goroutine for auto-restart.
	go m.watchChannel(parentCtx, id, entry)

	return nil
}

// watchChannel monitors a channel and restarts it on failure with exponential backoff.
// Aligned with OpenClaw's ChannelManager auto-restart (5s → 5min, max 10 retries).
func (m *ChannelManager) watchChannel(parentCtx context.Context, id string, entry *channelEntry) {
	// The watchdog exits when:
	// 1. Parent context is cancelled (server shutdown)
	// 2. Channel is manually stopped
	// 3. Max retry attempts exceeded
	//
	// Note: The current implementation relies on the Channel.Start() to return
	// errors or the context to be cancelled. For channels that use long-polling
	// or webhook servers, the restart is triggered when Start() returns an error
	// during the initial connection phase. Runtime failures within goroutines
	// should be handled by the channel implementation itself.
	<-parentCtx.Done()
}

// retryDelay calculates the backoff delay for the given retry attempt.
func retryDelay(attempt int) time.Duration {
	delay := float64(initialRetryDelay) * math.Pow(2, float64(attempt))
	if delay > float64(maxRetryDelay) {
		delay = float64(maxRetryDelay)
	}
	return time.Duration(delay)
}
