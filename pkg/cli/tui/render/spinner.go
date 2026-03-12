// Package render provides output and rendering components for the TUI.
//
// It includes a streaming renderer, markdown formatting, spinner
// animation, tool-call panels, and the welcome banner.
package render

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/muesli/termenv"
)

// Spinner frames — braille dot pattern (same as Claude Code).
var defaultFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner displays an animated indicator while waiting for a response.
//
// It renders a single line like:
//
//	⠹ Thinking...          (first 2 seconds)
//	⠼ Thinking... (3.2s)   (after 2 seconds, shows elapsed time)
//
// The Spinner is goroutine-safe; [Start] and [Stop] may be called from
// different goroutines.
type Spinner struct {
	frames   []string
	interval time.Duration
	label    string
	output   *termenv.Output

	mu      sync.Mutex
	running bool
	start   time.Time
	done    chan struct{}
}

// SpinnerOption configures a Spinner.
type SpinnerOption func(*Spinner)

// WithFrames sets custom animation frames.
func WithFrames(frames []string) SpinnerOption {
	return func(s *Spinner) { s.frames = frames }
}

// WithInterval sets the animation frame interval.
func WithInterval(d time.Duration) SpinnerOption {
	return func(s *Spinner) { s.interval = d }
}

// WithOutput sets a custom termenv output (useful for testing).
func WithOutput(o *termenv.Output) SpinnerOption {
	return func(s *Spinner) { s.output = o }
}

// NewSpinner creates a Spinner with the given label.
func NewSpinner(label string, opts ...SpinnerOption) *Spinner {
	s := &Spinner{
		frames:   defaultFrames,
		interval: 80 * time.Millisecond,
		label:    label,
		output:   termenv.NewOutput(os.Stdout),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins the spinner animation in a background goroutine.
// It is safe to call Start on an already-running spinner (no-op).
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.start = time.Now()
	s.done = make(chan struct{})
	s.mu.Unlock()

	go s.run()
}

// Stop halts the spinner and clears its line from the terminal.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.done)
	s.mu.Unlock()

	// Clear the spinner line.
	s.clearLine()
}

// IsRunning reports whether the spinner is currently animating.
func (s *Spinner) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// run is the animation loop.
func (s *Spinner) run() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	idx := 0
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.renderFrame(idx)
			idx = (idx + 1) % len(s.frames)
		}
	}
}

// renderFrame writes a single spinner frame to the terminal.
func (s *Spinner) renderFrame(idx int) {
	frame := s.frames[idx]
	elapsed := time.Since(s.start)

	style := s.output.String(frame + " " + s.label)
	style = style.Foreground(s.output.Color("241")) // gray

	var line string
	if elapsed > 2*time.Second {
		// Show elapsed time after 2 seconds.
		line = fmt.Sprintf("\r%s (%0.1fs)", style, elapsed.Seconds())
	} else {
		line = fmt.Sprintf("\r%s", style)
	}

	// Overwrite current line.
	fmt.Fprint(os.Stdout, line)
	s.output.ClearLineRight()
}

// clearLine moves the cursor to the beginning and erases the line.
func (s *Spinner) clearLine() {
	fmt.Fprint(os.Stdout, "\r")
	s.output.ClearLineRight()
}
