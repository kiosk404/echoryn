// Package terminal provides low-level terminal state management.
//
// It wraps golang.org/x/term to offer a clean lifecycle for raw mode,
// terminal size queries, and SIGWINCH resize handling. All terminal
// operations go through a single [Terminal] instance so that the rest
// of the TUI never touches raw ANSI escapes or os.Stdin.Fd() directly.
package terminal

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/term"
)

// Terminal manages the lifecycle of the controlling terminal.
//
// It owns the file descriptor and the saved cooked-mode state, and
// provides goroutine-safe access to width/height. A single Terminal
// should be created at TUI startup and its [Close] method deferred.
type Terminal struct {
	fd       int
	oldState *term.State

	mu     sync.RWMutex
	width  int
	height int

	// resizeCb is invoked (in a separate goroutine) when SIGWINCH fires.
	resizeCb func(width, height int)
}

// New creates a Terminal for the given file (typically os.Stdin).
// It does NOT enter raw mode — call [Terminal.EnterRawMode] explicitly.
func New(f *os.File) *Terminal {
	t := &Terminal{
		fd: int(f.Fd()),
	}
	t.refreshSize()
	return t
}

// EnterRawMode switches the terminal to raw mode and saves the original
// state for later restoration. It is safe to call multiple times; only
// the first call takes effect.
func (t *Terminal) EnterRawMode() error {
	if t.oldState != nil {
		return nil // already in raw mode
	}

	old, err := term.MakeRaw(t.fd)
	if err != nil {
		return fmt.Errorf("terminal: enter raw mode: %w", err)
	}
	t.oldState = old
	return nil
}

// Restore returns the terminal to its original (cooked) mode.
// It is safe to call multiple times or on an already-restored terminal.
func (t *Terminal) Restore() {
	if t.oldState != nil {
		_ = term.Restore(t.fd, t.oldState)
		t.oldState = nil
	}
}

// Close is an alias for [Restore], suitable for use with defer.
func (t *Terminal) Close() {
	t.Restore()
}

// IsRaw reports whether the terminal is currently in raw mode.
func (t *Terminal) IsRaw() bool {
	return t.oldState != nil
}

// Fd returns the underlying file descriptor.
func (t *Terminal) Fd() int {
	return t.fd
}

// Size returns the current terminal width and height in columns/rows.
// The values are cached and updated on SIGWINCH.
func (t *Terminal) Size() (width, height int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.width, t.height
}

// Width is a convenience helper that returns only the column count.
func (t *Terminal) Width() int {
	w, _ := t.Size()
	return w
}

// Height is a convenience helper that returns only the row count.
func (t *Terminal) Height() int {
	_, h := t.Size()
	return h
}

// OnResize registers a callback that fires whenever the terminal size
// changes. Only one callback is supported — subsequent calls overwrite
// the previous one.
func (t *Terminal) OnResize(fn func(width, height int)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resizeCb = fn
}

// refreshSize queries the kernel for the current terminal dimensions
// and updates the cached width/height.
func (t *Terminal) refreshSize() {
	w, h, err := term.GetSize(t.fd)
	if err != nil || w <= 0 {
		w = 80
	}
	if err != nil || h <= 0 {
		h = 24
	}

	t.mu.Lock()
	t.width = w
	t.height = h
	cb := t.resizeCb
	t.mu.Unlock()

	if cb != nil {
		go cb(w, h)
	}
}
