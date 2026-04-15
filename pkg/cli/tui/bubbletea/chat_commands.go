package bubbletea

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kiosk404/echoryn/pkg/cli/tui/render"
)

// ─────────────────────────────────────────────────────────────────────────────
// tea.Cmd factories — Command Pattern.
//
// Each function returns a tea.Cmd that performs an async operation
// and delivers the result as a tea.Msg to the Update() loop.
// ─────────────────────────────────────────────────────────────────────────────

// startRealtimeStreamCmd creates a streaming command that delivers deltas
// in real-time via the BubbleTea program's Send method.
//
// This is the preferred approach: a goroutine reads from the SSE stream
// and sends each delta as a tea.Msg to the program, enabling real-time
// View() updates during streaming.
func startRealtimeStreamCmd(p *tea.Program, client ChatClient, messages []ChatMessage, timeout time.Duration) context.CancelFunc {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	go func() {
		result, err := client.ChatStream(ctx, messages,
			func(delta string) {
				p.Send(StreamDeltaMsg{Delta: delta})
			},
			func(toolName string) {
				p.Send(StreamToolCallMsg{Name: toolName})
			},
		)

		if err != nil {
			p.Send(StreamDoneMsg{Err: err})
			return
		}

		msg := StreamDoneMsg{}
		if result != nil {
			msg.Content = result.Content
			if result.Usage != nil {
				msg.Usage = &ChatTokenUsage{
					PromptTokens:     result.Usage.PromptTokens,
					CompletionTokens: result.Usage.CompletionTokens,
					TotalTokens:      result.Usage.TotalTokens,
				}
			}
		}
		p.Send(msg)
	}()

	return cancel
}

// spinnerTickCmd returns a tea.Cmd that sends SpinnerTickMsg after 80ms.
func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg{Time: t}
	})
}

// renderMarkdownCmd creates a tea.Cmd that renders markdown off the main thread.
func renderMarkdownCmd(content string, width int) tea.Cmd {
	return func() tea.Msg {
		md := render.NewMarkdownRenderer(width-4, render.DetectColorProfile())
		rendered := md.Render(content)
		return RenderDoneMsg{
			Rendered: rendered,
			Raw:      content,
		}
	}
}
