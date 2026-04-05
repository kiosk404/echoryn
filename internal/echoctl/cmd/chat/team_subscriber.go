package chat

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/kiosk404/echoryn/pkg/cli/tui/command"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// TeamHTTPSubscriber implements command.TeamEventSubscriber over HTTP SSE.
//
// It connects to GET /v1/teams/:id/events and reads Server-Sent Events,
// converting them to command.TeamEvent and pushing them to a channel.
//
// This implementation is used by both TUI and any future CLI-based consumer.
// GUI clients may use a different implementation (e.g., WebSocket).
type TeamHTTPSubscriber struct {
	baseURL    string
	sessionKey string
	httpClient *http.Client
}

// Ensure TeamHTTPSubscriber implements command.TeamEventSubscriber at compile time.
var _ command.TeamEventSubscriber = (*TeamHTTPSubscriber)(nil)

// NewTeamHTTPSubscriber creates a new TeamHTTPSubscriber.
func NewTeamHTTPSubscriber(baseURL, sessionKey string, httpClient *http.Client) *TeamHTTPSubscriber {
	return &TeamHTTPSubscriber{
		baseURL:    strings.TrimRight(baseURL, "/"),
		sessionKey: sessionKey,
		httpClient: httpClient,
	}
}

// Subscribe opens an SSE connection and returns a channel of team events.
// The channel is closed when the context is cancelled or the connection ends.
func (s *TeamHTTPSubscriber) Subscribe(ctx context.Context, teamID string) (<-chan command.TeamEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/v1/teams/"+teamID+"/events", nil)
	if err != nil {
		return nil, fmt.Errorf("create SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if s.sessionKey != "" {
		req.Header.Set("X-Session-Key", s.sessionKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SSE connect: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("SSE connect: server returned %d", resp.StatusCode)
	}

	ch := make(chan command.TeamEvent, 64)
	go s.readLoop(ctx, resp, ch)
	return ch, nil
}

// readLoop reads SSE lines from the HTTP response and parses them into TeamEvent.
// It follows the SSE spec: "event: <type>\ndata: <json>\n\n"
func (s *TeamHTTPSubscriber) readLoop(ctx context.Context, resp *http.Response, ch chan<- command.TeamEvent) {
	defer resp.Body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(resp.Body)
	// Allow up to 64KB per line (generous for JSON payloads).
	scanner.Buffer(make([]byte, 0, 16*1024), 64*1024)

	var currentEventType string

	for scanner.Scan() {
		line := scanner.Text()

		// Parse SSE fields.
		switch {
		case strings.HasPrefix(line, "event: "):
			currentEventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := strings.TrimPrefix(line, "data: ")

			// Skip the initial "connected" event (not a team lifecycle event).
			if currentEventType == "connected" {
				currentEventType = ""
				continue
			}

			var event command.TeamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				logger.Warn("[TeamHTTPSubscriber] failed to parse event: %v (data: %s)", err, data)
				currentEventType = ""
				continue
			}

			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
			currentEventType = ""
		case line == "":
			// Empty line = event boundary (already handled above).
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		// Only log if context wasn't cancelled (normal shutdown).
		select {
		case <-ctx.Done():
		default:
			logger.Warn("[TeamHTTPSubscriber] SSE read error: %v", err)
		}
	}
}
