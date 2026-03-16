// Package completion provides auto-completion for commands and suggestions.
package completion

import (
	"sort"
	"strings"
)

// Completion represents a completion suggestion.
type Completion struct {
	Value       string // The completed text
	Display     string // Display text (if different from value)
	Description string // Optional description
	MatchIndex  int    // Index where match starts
}

// Provider provides completion suggestions.
type Provider interface {
	Complete(prefix string) []Completion
}

// Manager manages multiple completion providers.
type Manager struct {
	providers map[string]Provider
	commands  []Command
}

// Command represents a slash command for completion.
type Command struct {
	Name        string
	Description string
	Aliases     []string
	Arguments   []ArgSpec
}

// ArgSpec describes a command argument.
type ArgSpec struct {
	Name        string
	Description string
	Required    bool
	Options     []string // Valid options if limited
}

// NewManager creates a new completion manager.
func NewManager() *Manager {
	return &Manager{
		providers: make(map[string]Provider),
		commands:  []Command{},
	}
}

// RegisterProvider registers a completion provider.
func (m *Manager) RegisterProvider(name string, p Provider) {
	m.providers[name] = p
}

// RegisterCommand registers a slash command.
func (m *Manager) RegisterCommand(cmd Command) {
	m.commands = append(m.commands, cmd)
}

// Complete returns completions for the given input.
func (m *Manager) Complete(input string, cursorCol int) []Completion {
	if input == "" {
		return nil
	}

	// Get the text before cursor
	prefix := input[:min(cursorCol, len(input))]

	// Check for slash command
	if strings.HasPrefix(prefix, "/") {
		return m.completeSlashCommand(prefix)
	}

	// Check for file path
	if strings.Contains(prefix, "@") {
		return m.completeFilePath(prefix)
	}

	// Use registered providers
	var results []Completion
	for _, p := range m.providers {
		completions := p.Complete(prefix)
		results = append(results, completions...)
	}

	return results
}

// completeSlashCommand returns completions for slash commands.
func (m *Manager) completeSlashCommand(prefix string) []Completion {
	var results []Completion

	// Extract command part after /
	cmdPart := prefix[1:]

	// Check if completing command arguments
	if spaceIdx := strings.Index(cmdPart, " "); spaceIdx >= 0 {
		cmdName := cmdPart[:spaceIdx]
		args := cmdPart[spaceIdx+1:]

		// Find matching command
		for _, cmd := range m.commands {
			if cmd.Name == cmdName || cmd.Name == "/"+cmdName {
				return m.completeCommandArgs(cmd, args, prefix)
			}
			for _, alias := range cmd.Aliases {
				if alias == cmdName || alias == "/"+cmdName {
					return m.completeCommandArgs(cmd, args, prefix)
				}
			}
		}
		return nil
	}

	// Complete command names
	for _, cmd := range m.commands {
		cmdName := strings.TrimPrefix(cmd.Name, "/")
		if strings.HasPrefix(cmdName, cmdPart) {
			results = append(results, Completion{
				Value:       "/" + cmdName + " ",
				Display:     "/" + cmdName,
				Description: cmd.Description,
				MatchIndex:  len(cmdPart),
			})
		}

		// Also match aliases
		for _, alias := range cmd.Aliases {
			aliasName := strings.TrimPrefix(alias, "/")
			if strings.HasPrefix(aliasName, cmdPart) {
				results = append(results, Completion{
					Value:       "/" + aliasName + " ",
					Display:     "/" + aliasName,
					Description: cmd.Description + " (alias for /" + cmdName + ")",
					MatchIndex:  len(cmdPart),
				})
			}
		}
	}

	// Sort by name
	sort.Slice(results, func(i, j int) bool {
		return results[i].Display < results[j].Display
	})

	return results
}

// completeCommandArgs returns completions for command arguments.
func (m *Manager) completeCommandArgs(cmd Command, args string, prefix string) []Completion {
	// If command has no argument specs, nothing to complete
	if len(cmd.Arguments) == 0 {
		return nil
	}

	// Parse current argument position
	parts := strings.Fields(args)
	argIdx := len(parts)
	if strings.HasSuffix(args, " ") {
		argIdx++
	}

	// Check bounds before accessing cmd.Arguments.
	if argIdx <= 0 || argIdx > len(cmd.Arguments) {
		return nil
	}

	arg := cmd.Arguments[argIdx-1]
	if len(arg.Options) == 0 {
		return nil
	}
	var results []Completion

	for _, opt := range arg.Options {
		// Match against current partial
		if len(parts) > 0 {
			partial := parts[len(parts)-1]
			if strings.HasPrefix(opt, partial) || partial == "" {
				results = append(results, Completion{
					Value:       opt,
					Display:     opt,
					Description: arg.Description,
				})
			}
		} else {
			results = append(results, Completion{
				Value:       opt,
				Display:     opt,
				Description: arg.Description,
			})
		}
	}

	return results
}

// completeFilePath returns completions for file paths.
func (m *Manager) completeFilePath(prefix string) []Completion {
	// Extract the @file part
	atIdx := strings.LastIndex(prefix, "@")
	if atIdx < 0 {
		return nil
	}

	filePart := prefix[atIdx+1:]

	// Use file provider if available
	if fp, ok := m.providers["file"]; ok {
		completions := fp.Complete(filePart)
		// Prepend @ to values
		for i := range completions {
			completions[i].Value = "@" + completions[i].Value
		}
		return completions
	}

	return nil
}

// =============================================================================
// Default Commands
// =============================================================================

// DefaultCommands returns the default set of slash commands.
func DefaultCommands() []Command {
	return []Command{
		{
			Name:        "help",
			Description: "Show help information",
			Aliases:     []string{"h", "?"},
		},
		{
			Name:        "clear",
			Description: "Clear the conversation history",
			Aliases:     []string{"c"},
		},
		{
			Name:        "exit",
			Description: "Exit the application",
			Aliases:     []string{"quit", "q"},
		},
		{
			Name:        "team",
			Description: "Team management commands",
			Arguments: []ArgSpec{
				{
					Name:        "action",
					Description: "Action to perform",
					Required:    true,
					Options:     []string{"create", "join", "leave", "list", "status"},
				},
				{
					Name:        "template",
					Description: "Team template ID",
					Required:    false,
				},
			},
		},
		{
			Name:        "model",
			Description: "Change or show the current model",
			Arguments: []ArgSpec{
				{
					Name:        "model",
					Description: "Model to use",
					Required:    false,
				},
			},
		},
		{
			Name:        "config",
			Description: "Show or modify configuration",
			Arguments: []ArgSpec{
				{
					Name:        "key",
					Description: "Config key",
					Required:    false,
				},
			},
		},
	}
}
