package tui

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/kiosk404/echoryn/pkg/cli/tui/command"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// TeamEventHandler defines how team events are consumed.
// TUI implements this to print to terminal; GUI can implement to update UI state.
//
// This is the extension point for GUI support — any UI framework can implement
// this interface to receive real-time team events without depending on TUI internals.
type TeamEventHandler interface {
	// OnTeamEvent is called for each received team event.
	// Implementations must be goroutine-safe (called from a background goroutine).
	OnTeamEvent(event command.TeamEvent)
}

// teamEventWatcher manages the background SSE subscription goroutine.
// It is started when a team is created and stopped when the team is dissolved.
type teamEventWatcher struct {
	mu     sync.Mutex
	cancel context.CancelFunc

	subscriber command.TeamEventSubscriber
	handler    TeamEventHandler
}

// newTeamEventWatcher creates a new watcher (does not start it).
func newTeamEventWatcher(subscriber command.TeamEventSubscriber, handler TeamEventHandler) *teamEventWatcher {
	return &teamEventWatcher{
		subscriber: subscriber,
		handler:    handler,
	}
}

// Start begins watching events for the given team.
// If already watching, the previous subscription is stopped first.
func (w *teamEventWatcher) Start(ctx context.Context, teamID string) {
	w.Stop()

	if w.subscriber == nil {
		return
	}

	subCtx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancel = cancel
	w.mu.Unlock()

	events, err := w.subscriber.Subscribe(subCtx, teamID)
	if err != nil {
		logger.Warn("[TUI] failed to subscribe to team events: %v", err)
		cancel()
		return
	}

	go func() {
		for event := range events {
			w.handler.OnTeamEvent(event)
		}
	}()

	logger.Info("[TUI] team event watcher started for team %s", teamID)
}

// Stop cancels the current subscription (if any).
func (w *teamEventWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}

// tuiEventHandler implements TeamEventHandler for terminal output.
// It updates the local teamState and prints notifications to stdout.
type tuiEventHandler struct {
	mu        sync.Mutex
	teamState **command.TeamState // pointer to TUI's teamState pointer
}

func (h *tuiEventHandler) OnTeamEvent(event command.TeamEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Update local teamState member status.
	if *h.teamState != nil && event.MemberID != "" && event.MemberStatus != "" {
		for i, m := range (*h.teamState).Members {
			if m.ID == event.MemberID {
				(*h.teamState).Members[i].Status = event.MemberStatus
				break
			}
		}
	}

	// Print notification to terminal.
	icon := "📋"
	suffix := ""
	switch event.EventType {
	case "member_spawned":
		icon = "🚀"
		suffix = formatMemberInfo(event)
	case "member_started":
		icon = "▶️"
		suffix = formatMemberInfo(event)
	case "member_completed":
		icon = "✅"
		suffix = formatMemberInfo(event)
	case "member_failed":
		icon = "❌"
		suffix = formatMemberInfo(event)
	case "member_canceled":
		icon = "⏹️"
		suffix = formatMemberInfo(event)
	case "all_members_completed":
		icon = "🎉"
		suffix = "All team members completed"
		if event.Success != nil && !*event.Success {
			icon = "⚠️"
			suffix = "Team completed with failures"
		}
	case "team_dissolved":
		icon = "🔚"
		suffix = "Team dissolved"
	default:
		suffix = event.EventType
	}

	// Write to stdout with carriage return to handle mid-input display.
	fmt.Fprintf(os.Stdout, "\r%s [Team] %s\n", icon, suffix)
	os.Stdout.Sync()
}

// formatMemberInfo builds a display string for member-level events.
func formatMemberInfo(event command.TeamEvent) string {
	label := event.MemberLabel
	if label == "" {
		label = event.MemberID
	}
	parts := []string{label}
	if event.MemberRole != "" {
		parts = append(parts, fmt.Sprintf("(%s)", event.MemberRole))
	}
	if event.MemberStatus != "" {
		parts = append(parts, fmt.Sprintf("[%s]", event.MemberStatus))
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}
