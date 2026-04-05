package team

import (
	"sync"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// ChannelTeamPublisher implements TeamPublisher by broadcasting events
// to per-team subscriber channels.
//
// Design:
//   - Each team can have multiple SSE subscribers (e.g., TUI, GUI, monitoring dashboards).
//   - Events are non-blocking: if a subscriber channel is full, the event is dropped with a warning.
//   - Thread-safe: all operations are protected by a RWMutex.
//   - The Subscribe/Unsubscribe contract follows the observer pattern.
//
// Extension points:
//   - GUI clients subscribe via the same SSE HTTP endpoint.
//   - In-process consumers (e.g., plugins) can call Subscribe() directly without going through HTTP.
type ChannelTeamPublisher struct {
	mu          sync.RWMutex
	subscribers map[string][]*subscription // teamID → subscriber list
}

// subscription wraps a subscriber channel with an ID for safe unsubscribe.
type subscription struct {
	id uint64
	ch chan *TeamEvent
}

var nextSubID uint64
var subIDMu sync.Mutex

func nextSubscriptionID() uint64 {
	subIDMu.Lock()
	defer subIDMu.Unlock()
	nextSubID++
	return nextSubID
}

// NewChannelTeamPublisher creates a new ChannelTeamPublisher.
func NewChannelTeamPublisher() *ChannelTeamPublisher {
	return &ChannelTeamPublisher{
		subscribers: make(map[string][]*subscription),
	}
}

// PublishTeamEvent broadcasts an event to all subscribers of the event's team.
// Implements TeamPublisher interface.
func (p *ChannelTeamPublisher) PublishTeamEvent(event *TeamEvent) {
	if event == nil {
		return
	}
	p.mu.RLock()
	subs := p.subscribers[event.TeamID]
	p.mu.RUnlock()

	for _, sub := range subs {
		select {
		case sub.ch <- event:
		default:
			logger.Warn("[TeamPublisher] subscriber %d channel full for team %s, dropping event %s",
				sub.id, event.TeamID, event.EventType)
		}
	}
}

// Subscribe creates a new event channel for a team.
// Returns a read-only channel and an unsubscribe function.
//
// Usage:
//
//	ch, unsub := publisher.Subscribe(teamID)
//	defer unsub()
//	for event := range ch { ... }
//
// The channel is closed when Unsubscribe is called (via the returned function)
// or when the publisher is shut down.
func (p *ChannelTeamPublisher) Subscribe(teamID string) (<-chan *TeamEvent, func()) {
	sub := &subscription{
		id: nextSubscriptionID(),
		ch: make(chan *TeamEvent, 64),
	}

	p.mu.Lock()
	p.subscribers[teamID] = append(p.subscribers[teamID], sub)
	total := len(p.subscribers[teamID])
	p.mu.Unlock()

	logger.Info("[TeamPublisher] new subscriber %d for team %s (total: %d)", sub.id, teamID, total)

	unsub := func() {
		p.unsubscribe(teamID, sub)
	}
	return sub.ch, unsub
}

// unsubscribe removes a subscriber and closes its channel.
func (p *ChannelTeamPublisher) unsubscribe(teamID string, target *subscription) {
	p.mu.Lock()
	defer p.mu.Unlock()

	subs := p.subscribers[teamID]
	for i, sub := range subs {
		if sub.id == target.id {
			// Remove by swapping with last element (order doesn't matter).
			subs[i] = subs[len(subs)-1]
			p.subscribers[teamID] = subs[:len(subs)-1]
			close(sub.ch)
			break
		}
	}
	if len(p.subscribers[teamID]) == 0 {
		delete(p.subscribers, teamID)
	}

	logger.Info("[TeamPublisher] unsubscribed %d from team %s", target.id, teamID)
}

// SubscriberCount returns the number of active subscribers for a team. (diagnostic use)
func (p *ChannelTeamPublisher) SubscriberCount(teamID string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.subscribers[teamID])
}

var _ TeamPublisher = (*ChannelTeamPublisher)(nil)
