// Package tui provides the interactive terminal user interface for the
// Echoryn chat command.
//
// It integrates reeflective/readline for line editing, charmbracelet
// glamour/lipgloss/termenv for rendering, and a pluggable slash-command
// framework. The design favours scroll-back friendly output (no alt-screen)
// and aims for a Claude Code-like user experience.
package tui

import (
	"time"

	"github.com/kiosk404/echoryn/pkg/cli/tui/input"
)

// Config holds all configurable options for the TUI.
type Config struct {
	// Prompt is the primary input prompt (default: "> ").
	Prompt string

	// MultilinePrompt is shown on continuation lines (default: "· ").
	MultilinePrompt string

	// History configures persistent command history.
	History input.HistoryConfig

	// RequestTimeout is the maximum duration for a single chat request.
	RequestTimeout time.Duration
}

// DefaultConfig returns a Config with production-ready defaults.
func DefaultConfig() Config {
	return Config{
		Prompt:          "\033[1m\033[38;5;208m> \033[0m", // bold orange "> "
		MultilinePrompt: "\033[38;5;241m· \033[0m",        // gray "· "
		RequestTimeout:  120 * time.Second,
	}
}

// Option is a functional option for configuring the TUI.
type Option func(*Config)

// WithPrompt sets a custom primary prompt.
func WithPrompt(prompt string) Option {
	return func(c *Config) { c.Prompt = prompt }
}

// WithRequestTimeout sets the per-request timeout.
func WithRequestTimeout(d time.Duration) Option {
	return func(c *Config) { c.RequestTimeout = d }
}

// WithHistoryFile sets the path for persistent history.
func WithHistoryFile(path string) Option {
	return func(c *Config) { c.History.FilePath = path }
}
