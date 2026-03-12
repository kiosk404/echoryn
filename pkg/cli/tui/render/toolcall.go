package render

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/muesli/termenv"
)

// ToolStatus represents the current state of a tool invocation.
type ToolStatus int

const (
	ToolRunning ToolStatus = iota
	ToolSuccess
	ToolFailed
)

// toolCallEntry is an individual tool invocation record.
type toolCallEntry struct {
	Name      string
	Status    ToolStatus
	StartTime time.Time
	Duration  time.Duration
}

// ToolCallPanel manages the display of tool invocations during a
// streaming response. It tracks multiple concurrent tool calls and
// renders them as a compact list:
//
//	⚡ web_search          ✓ 1.2s
//	⚡ memory_write         ⏳ running...
//	⚡ code_edit            ✗ failed
type ToolCallPanel struct {
	output *termenv.Output

	mu    sync.Mutex
	calls []toolCallEntry
	lines int // number of lines currently rendered
}

// NewToolCallPanel creates a new panel with default output.
func NewToolCallPanel() *ToolCallPanel {
	return &ToolCallPanel{
		output: termenv.NewOutput(os.Stdout),
	}
}

// Start records the beginning of a tool call and renders it.
func (p *ToolCallPanel) Start(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, toolCallEntry{
		Name:      name,
		Status:    ToolRunning,
		StartTime: time.Now(),
	})

	p.renderLastCall()
}

// Finish marks a tool call as completed (success or failure) and
// re-renders the line.
func (p *ToolCallPanel) Finish(name string, success bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find the most recent running call with this name.
	for i := len(p.calls) - 1; i >= 0; i-- {
		if p.calls[i].Name == name && p.calls[i].Status == ToolRunning {
			p.calls[i].Duration = time.Since(p.calls[i].StartTime)
			if success {
				p.calls[i].Status = ToolSuccess
			} else {
				p.calls[i].Status = ToolFailed
			}
			break
		}
	}

	// We don't re-render inline for simplicity; the final state is
	// visible when the streaming completes and gets re-rendered.
}

// renderLastCall appends a new tool-call indicator line to the terminal.
func (p *ToolCallPanel) renderLastCall() {
	if len(p.calls) == 0 {
		return
	}

	entry := p.calls[len(p.calls)-1]
	line := p.formatEntry(entry)
	fmt.Fprintln(os.Stdout, line)
	p.lines++
}

// formatEntry produces the ANSI-styled string for a single entry.
func (p *ToolCallPanel) formatEntry(e toolCallEntry) string {
	var sb strings.Builder

	// Lightning bolt icon.
	icon := p.output.String("⚡").Foreground(p.output.Color("214")).String()
	sb.WriteString(icon)
	sb.WriteString(" ")

	// Tool name.
	name := p.output.String(e.Name).Foreground(p.output.Color("241")).String()
	sb.WriteString(name)

	// Status indicator.
	sb.WriteString("  ")
	switch e.Status {
	case ToolRunning:
		status := p.output.String("⏳ running...").Foreground(p.output.Color("241")).String()
		sb.WriteString(status)
	case ToolSuccess:
		dur := fmt.Sprintf("✓ %.1fs", e.Duration.Seconds())
		status := p.output.String(dur).Foreground(p.output.Color("82")).String() // green
		sb.WriteString(status)
	case ToolFailed:
		status := p.output.String("✗ failed").Foreground(p.output.Color("196")).String() // red
		sb.WriteString(status)
	}

	return sb.String()
}

// LineCount returns the number of terminal lines this panel has output.
func (p *ToolCallPanel) LineCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lines
}

// Reset clears all tracked tool calls (but does not erase terminal output).
func (p *ToolCallPanel) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = p.calls[:0]
	p.lines = 0
}
