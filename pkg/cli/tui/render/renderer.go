package render

import (
	"fmt"
	"os"
	"strings"
	"sync"

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
// Usage:
//
//	r := render.NewStreamRenderer(width)
//	r.StartThinking()
//	// ... on each SSE delta:
//	r.OnDelta("Hello")
//	// ... on tool call:
//	r.OnToolCall("web_search")
//	// ... when stream ends:
//	rendered := r.Finish()
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

	// Ensure the raw streaming output ends with a newline so the
	// cursor is on a fresh line before we start overwriting.
	needsNewline := !strings.HasSuffix(content, "\n")
	if needsNewline {
		fmt.Fprintln(os.Stdout)
	}

	// --- Overwrite raw output with rendered markdown ---
	//
	// 1. Calculate how many visual lines the raw text occupied on screen.
	//    If we added a newline above, the text effectively has an extra
	//    blank line — account for it.
	// 2. Move cursor up that many lines.
	// 3. Erase from cursor to end-of-screen.
	// 4. Print the markdown-rendered version.
	textForCounting := content
	if needsNewline {
		textForCounting += "\n"
	}
	lines := countVisualLines(textForCounting, r.width)
	if lines > 0 {
		// CUU – Cursor Up by `lines` rows.
		fmt.Fprintf(os.Stdout, "\x1b[%dA", lines)
	}
	// Move to column 0, then erase from cursor to end of display.
	fmt.Fprint(os.Stdout, "\r\x1b[J")

	rendered := r.md.Render(content)
	fmt.Fprintln(os.Stdout, rendered)

	r.state = stateDone
	return content
}

// countVisualLines calculates the number of screen lines that text would
// occupy in a terminal of the given width, accounting for word-wrap and
// wide (CJK) characters.
// Each logical line (delimited by \n) may span multiple screen rows if it
// is wider than the terminal.
func countVisualLines(text string, termWidth int) int {
	if termWidth <= 0 {
		termWidth = 80
	}
	lines := strings.Split(text, "\n")
	total := 0
	for _, line := range lines {
		w := runewidth.StringWidth(line)
		if w == 0 {
			total++ // empty line still occupies one screen row
			continue
		}
		// Number of screen rows = ceil(w / termWidth).
		rows := (w + termWidth - 1) / termWidth
		total += rows
	}
	// Subtract 1 because the cursor sits at the end of the last printed
	// character, which is already on the last line — we don't need to
	// move up past it.
	if total > 0 {
		total--
	}
	return total
}

// Abort stops the renderer without re-rendering markdown.
// Returns whatever content has been accumulated so far.
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
