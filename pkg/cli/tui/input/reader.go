package input

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/reeflective/readline"
)

// Reader wraps a reeflective/readline Shell to provide a high-level
// line-reading API for the TUI main loop.
//
// It handles prompt rendering, multiline editing (Shift+Enter or
// trailing backslash), tab completion, and persistent history — all
// through a single [ReadLine] call.
type Reader struct {
	shell     *readline.Shell
	completer *Completer
	cfg       Config
}

// Config holds all configuration for the input [Reader].
type Config struct {
	// Prompt is the primary prompt string (default: "> ").
	Prompt string

	// MultilinePrompt is shown on continuation lines (default: "· ").
	MultilinePrompt string

	// History configures persistent command history.
	History HistoryConfig
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Prompt:          "> ",
		MultilinePrompt: "· ",
		History:         HistoryConfig{},
	}
}

// NewReader creates a new input Reader with the given configuration.
// The caller should call [Reader.Close] when done.
func NewReader(cfg Config, commands []CommandInfo) (*Reader, error) {
	shell := readline.NewShell()

	r := &Reader{
		shell:     shell,
		completer: NewCompleter(commands),
		cfg:       cfg,
	}

	r.configureShell()

	if err := setupHistory(shell, cfg.History); err != nil {
		// History setup failure is not fatal; log and continue.
		_ = err
	}

	return r, nil
}

// configureShell sets up the readline shell with prompts, multiline
// behaviour, and completion.
func (r *Reader) configureShell() {
	prompt := r.cfg.Prompt
	if prompt == "" {
		prompt = "> "
	}
	multiPrompt := r.cfg.MultilinePrompt
	if multiPrompt == "" {
		multiPrompt = "· "
	}

	// Primary prompt (shown on the first line).
	r.shell.Prompt.Primary(func() string {
		return prompt
	})

	// Secondary prompt (shown on continuation lines).
	r.shell.Prompt.Secondary(func() string {
		return multiPrompt
	})

	// Multiline acceptance logic:
	// - Enter on a line ending with '\' → continue (strip the backslash, add newline)
	// - Enter on a non-empty line without trailing '\' → submit
	// - Enter on an empty line → submit (empty string is handled by caller)
	//
	// The reeflective/readline library also natively handles Shift+Enter
	// as "insert newline" when AcceptMultiline is set, so that path works
	// automatically.
	r.shell.AcceptMultiline = func(line []rune) bool {
		s := strings.TrimRightFunc(string(line), unicode.IsSpace)
		if strings.HasSuffix(s, "\\") {
			return false // keep reading; the shell removes the '\' and adds a newline
		}
		return true // accept the input
	}

	// Tab completion.
	r.shell.Completer = r.completer.Complete
}

// ReadLine displays the prompt and reads one (possibly multi-line)
// user input. It returns the trimmed input string.
//
// Errors:
//   - [io.EOF] when the user presses Ctrl-D on an empty line.
//   - [readline.ErrInterrupt] on Ctrl-C.
func (r *Reader) ReadLine() (string, error) {
	// Ensure cursor is at column 0 before readline draws the prompt.
	fmt.Fprint(os.Stdout, "\r")

	// Drain the kernel output queue so all prior writes (Markdown
	// re-render, ANSI sequences, etc.) have been transmitted to the
	// terminal emulator via the pty master fd.
	drainStdout()

	// Give the terminal emulator a moment to read and render the
	// output that tcdrain just pushed into the pty.
	//
	// Background: readline's Refresh() sends a DSR cursor-position
	// query (\x1b[6n) and blocks on os.Stdin.Read() waiting for the
	// CPR response. If the terminal emulator hasn't finished reading
	// our prior output from the pty master, the DSR query can sit
	// behind that data in the pty buffer and the CPR reply arrives
	// late — readline blocks and the prompt is invisible until the
	// user presses a key (the keypress "unsticks" the Read).
	//
	// tcdrain guarantees that data has reached the pty master, but
	// the terminal emulator's read-from-master → render pipeline
	// may lag by a few milliseconds. A short sleep is enough to let
	// it catch up without any of the pitfalls of a DSR-based sync
	// (which would require raw-mode stdin reads that can consume user
	// keystrokes or block indefinitely).
	time.Sleep(50 * time.Millisecond)

	line, err := r.shell.Readline()
	if err != nil {
		if errors.Is(err, readline.ErrInterrupt) {
			return "", readline.ErrInterrupt
		}
		if errors.Is(err, io.EOF) {
			return "", io.EOF
		}
		return "", err
	}

	return line, nil
}

// SetCommands updates the slash-command list used by the completer.
func (r *Reader) SetCommands(commands []CommandInfo) {
	r.completer.SetCommands(commands)
}

// Shell returns the underlying readline shell for advanced configuration.
// Most callers should not need this.
func (r *Reader) Shell() *readline.Shell {
	return r.shell
}

// Close performs any cleanup needed by the Reader.
func (r *Reader) Close() {
	// Currently a no-op; reserved for future resource cleanup.
}
