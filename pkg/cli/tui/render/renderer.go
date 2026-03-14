package render

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

// renderState tracks the current phase of a streaming response.
type renderState int

const (
	stateIdle      renderState = iota
	stateThinking              // spinner is running
	stateStreaming             // receiving text deltas
	stateRendering             // re-rendering markdown
	stateDone
)

// StreamRenderer manages the rendering lifecycle of a single assistant
// response. It coordinates the spinner, raw text streaming, tool-call
// panel, and final markdown re-render.
//
// The output flow mirrors Gemini CLI.
//
// 1. Show "Thinking..." spinner.
// 2. On first delta: stop spinner
// 3. On tool call: render compact tool indicator
// 4. On finish: overwrite raw text with glamour-rendered markdown.
type StreamRenderer struct {
	md      *MarkdownRenderer
	spinner *Spinner
	tools   *ToolCallPanel
	width   int

	mu       sync.Mutex
	state    renderState
	rawLines int
	content  strings.Builder
}

// NewStreamRenderer creates a renderer for the given terminal width.
func NewStreamRenderer(width int) *StreamRenderer {
	profile := termenv.NewOutput(os.Stdout).Profile
	return &StreamRenderer{
		md:      NewMarkdownRenderer(width-4, profile),
		spinner: NewSpinner("Thinking..."),
		tools:   NewToolCallPanel(),
		width:   width,
		state:   stateIdle,
	}
}

// StartThinking begins the spinner animation.
func (r *StreamRenderer) StartThinking() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state = stateThinking
	r.spinner.Start()
}

// OnDelta processes an incremental text chunk from the SSE stream.
// It stops the spinner on the first delta and prints the raw text.
func (r *StreamRenderer) OnDelta(delta string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state == stateThinking {
		r.spinner.Stop()
		r.state = stateStreaming
		// Print the assistant marker on the first delta.
		marker := lipgloss.NewStyle().
			Bold(true).
			Foreground(assistantColor).
			Render("✦ ")
		fmt.Fprint(os.Stdout, marker)
	}

	r.content.WriteString(delta)
	fmt.Fprint(os.Stdout, delta)

	// Track raw lines for later overwrite.
	r.rawLines += strings.Count(delta, "\n")
}

// OnToolCall records and displays a tool invocation during streaming.
func (r *StreamRenderer) OnToolCall(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state == stateThinking {
		r.spinner.Stop()
		r.state = stateStreaming
	}

	r.tools.Start(name)
}

// OnToolResult marks a tool call as finished.
func (r *StreamRenderer) OnToolResult(name string, success bool) {
	r.tools.Finish(name, success)
}

// Finish ends the streaming phase and re-renders the complete content
// as formatted markdown, overwriting the raw output.
// It returns the full raw content string for history purposes.
func (r *StreamRenderer) Finish() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure spinner is stopped.
	if r.state == stateThinking {
		r.spinner.Stop()
	}
	r.state = stateRendering

	content := r.content.String()
	if content == "" {
		r.state = stateDone
		return content
	}

	// Ensure the raw streaming output ends with a newline
	needsNewline := !strings.HasSuffix(content, "\n")
	if needsNewline {
		fmt.Fprintln(os.Stdout)
	}

	// --- Overwrite raw output with rendered markdown ---
	textForCounting := content
	if needsNewline {
		textForCounting += "\n"
	}
	// Add 1 for the "✦ " marker line prefix
	lines := countVisualLines(textForCounting, r.width)
	if lines > 0 {
		// CUU – Cursor Up by `lines` rows.
		fmt.Fprintf(os.Stdout, "\x1b[%dA", lines)
	}
	// Move to column 0, then erase from cursor to end of display.
	fmt.Fprint(os.Stdout, "\r\x1b[J")

	// Render with assistant marker.
	marker := lipgloss.NewStyle().
		Bold(true).
		Foreground(assistantColor).
		Render("✦ ")
	rendered := r.md.Render(content)
	fmt.Fprintln(os.Stdout, marker+rendered)

	r.state = stateDone
	return content
}

// FinishWithToolResult ends the stream and also renders ta tool result box.
func (r *StreamRenderer) FinishWithToolResult(toolName, desc, code string, success bool) string {
	content := r.Finish()

	// Render the tool result in a bordered box.
	box := RenderToolResult(toolName, desc, code, r.width, success)
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, box)

	return content
}

// countVisualLines calculates the number of screen lines that text would
// occupy in a terminal of the given width
func countVisualLines(text string, termWidth int) int {
	if termWidth <= 0 {
		termWidth = 80
	}
	lines := strings.Split(text, "\n")
	total := 0
	for _, line := range lines {
		w := runewidth.StringWidth(line)
		if w == 0 {
			total++
			continue
		}
		rows := (w + termWidth - 1) / termWidth
		total += rows
	}
	if total > 0 {
		total--
	}
	return total
}

// Abort stops the renderer without re-rendering markdown.
func (r *StreamRenderer) Abort() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state == stateThinking {
		r.spinner.Stop()
	}
	r.state = stateDone
	return r.content.String()
}

// OnError displays an error message and aborts the rendering.
func (r *StreamRenderer) OnError(err error) string {
	content := r.Abort()
	if content != "" {
		fmt.Fprintln(os.Stdout)
	}
	PrintError(err.Error())
	return content
}

// SetWidth updates the terminal width for subsequent renders.
func (r *StreamRenderer) SetWidth(width int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.width = width
	r.md.SetWidth(width - 4)
}

// Reset prepares the renderer for a new streaming response.
func (r *StreamRenderer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = stateIdle
	r.rawLines = 0
	r.content.Reset()
	r.tools.Reset()
}

// ─────────────────────────────────────────────────────────────────────────────
// Post-stream message rendering
// ─────────────────────────────────────────────────────────────────────────────

// PrintAwaitingMessage prints the "Awaiting your next command or request." line.
func PrintAwaitingMessage() {
	marker := lipgloss.NewStyle().Bold(true).Foreground(assistantColor).Render("✦ ")
	msg := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render("Awaiting your next command or request.")
	fmt.Println()
	fmt.Println(marker + msg)
}

// PrintContextInfo prints a context info line
func PrintContextInfo(info string) {
	if info == "" {
		return
	}
	fmt.Println()
	fmt.Println(subtleStyle.Render("  " + info))
}
