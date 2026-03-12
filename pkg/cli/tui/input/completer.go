package input

import (
	"strings"

	"github.com/reeflective/readline"
)

// CommandInfo describes a slash command for completion purposes.
type CommandInfo struct {
	Name        string
	Aliases     []string
	Description string
}

// Completer provides tab-completion for the readline shell.
// It supports slash command completion and can be extended with
// additional completion sources.
type Completer struct {
	commands []CommandInfo
}

// NewCompleter creates a new completer with the given command list.
func NewCompleter(commands []CommandInfo) *Completer {
	return &Completer{
		commands: commands,
	}
}

// Complete is the readline Completer function. It examines the current
// line and cursor position to produce relevant completions.
func (c *Completer) Complete(line []rune, cursor int) readline.Completions {
	text := string(line[:cursor])

	// Only complete slash commands at the beginning of the line.
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return readline.Completions{}
	}

	// Build completion candidates from registered commands.
	prefix := trimmed
	var values []string

	for _, cmd := range c.commands {
		candidate := "/" + cmd.Name
		if strings.HasPrefix(candidate, prefix) {
			values = append(values, candidate, cmd.Description)
		}
		for _, alias := range cmd.Aliases {
			aliasCandidate := "/" + alias
			if strings.HasPrefix(aliasCandidate, prefix) {
				values = append(values, aliasCandidate, cmd.Description+" (alias)")
			}
		}
	}

	if len(values) == 0 {
		return readline.Completions{}
	}

	return readline.CompleteValuesDescribed(values...).
		Tag("commands").
		NoSpace('/')
}

// SetCommands updates the command list used for completion.
// This allows the command registry to be updated after the completer is created.
func (c *Completer) SetCommands(commands []CommandInfo) {
	c.commands = commands
}
