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

	"github.com/kiosk404/echoryn/pkg/cli/tui/command"
)

// Config holds all configurable options for the TUI.
type Config struct {
	// ProgramName is the CLI binary name in resume hints
	ProgramName string

	// Prompt is the primary input prompt (default: "> ").
	Prompt string

	// MultilinePrompt is shown on continuation lines (default: "· ").
	MultilinePrompt string

	// RequestTimeout is the maximum duration for a single chat request.
	RequestTimeout time.Duration

	// TeamAPI provides the backend API for team operations (optional).
	TeamAPI command.TeamAPI
}

// DefaultConfig returns a Config with production-ready defaults.
func DefaultConfig() Config {
	return Config{
		ProgramName:     "echoryn",
		Prompt:          "> ", // bold blue "> "
		MultilinePrompt: ". ", // gray "· "
		RequestTimeout:  120 * time.Second,
	}
}

// Option is a functional option for configuring the TUI.
type Option func(*Config)

// WithPrompt sets a custom primary prompt.
func WithPrompt(prompt string) Option {
	return func(c *Config) { c.Prompt = prompt }
}

// WithProgramName sets the CLI binary name for resume hints.
func WithProgramName(name string) Option {
	return func(config *Config) { config.ProgramName = name }
}

// WithRequestTimeout sets the per-request timeout.
func WithRequestTimeout(d time.Duration) Option {
	return func(c *Config) { c.RequestTimeout = d }
}

// WithTeamAPI sets the team API for the TUI, enabling team commands.
func WithTeamAPI(api command.TeamAPI) Option {
	return func(c *Config) { c.TeamAPI = api }
}
