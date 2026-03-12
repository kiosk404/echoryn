// Package input provides the user input subsystem for the TUI.
//
// It integrates reeflective/readline as the line-editing backend and
// exposes a clean [Reader] API for the TUI main loop. Completion,
// multiline editing, and persistent history are all configured here.
package input

import (
	"os"
	"path/filepath"

	"github.com/reeflective/readline"
)

const (
	// defaultHistoryName is the name registered in readline's history sources.
	defaultHistoryName = "default"

	// defaultHistoryFile is the basename of the history file.
	defaultHistoryFile = ".echoctl_history"
)

// HistoryConfig holds options for the persistent command history.
type HistoryConfig struct {
	// FilePath overrides the default history file location.
	// If empty, defaults to $HOME/.echoctl_history.
	FilePath string

	// MaxEntries is the maximum number of history entries to keep.
	// Zero means unlimited.
	MaxEntries int
}

// historyFilePath returns the resolved path for the history file.
func historyFilePath(cfg HistoryConfig) string {
	if cfg.FilePath != "" {
		return cfg.FilePath
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, defaultHistoryFile)
}

// setupHistory configures the readline shell's history sources.
// It registers a file-backed history source so that past commands
// survive across sessions.
func setupHistory(shell *readline.Shell, cfg HistoryConfig) error {
	path := historyFilePath(cfg)

	// Ensure the parent directory exists.
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	// Register the file-backed history.
	shell.History.AddFromFile(defaultHistoryName, path)

	return nil
}
