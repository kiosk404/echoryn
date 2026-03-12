package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kiosk404/echoryn/pkg/cli/tui/input"
)

// Registry manages all registered slash commands.
//
// It provides lookup by name or alias, prefix-based completion,
// and formatted help output.
type Registry struct {
	commands map[string]Command // keyed by canonical name
	aliases  map[string]string  // alias → canonical name
	order    []string           // insertion order for deterministic help
}

// NewRegistry creates an empty command registry.
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
		aliases:  make(map[string]string),
	}
}

// Register adds a command to the registry.
// It panics if the name or any alias conflicts with an existing entry.
func (r *Registry) Register(cmd Command) {
	name := cmd.Name()
	if _, exists := r.commands[name]; exists {
		panic(fmt.Sprintf("command: duplicate registration of /%s", name))
	}
	if _, exists := r.aliases[name]; exists {
		panic(fmt.Sprintf("command: name /%s conflicts with existing alias", name))
	}

	r.commands[name] = cmd
	r.order = append(r.order, name)

	for _, alias := range cmd.Aliases() {
		if _, exists := r.commands[alias]; exists {
			panic(fmt.Sprintf("command: alias /%s conflicts with existing command", alias))
		}
		if _, exists := r.aliases[alias]; exists {
			panic(fmt.Sprintf("command: duplicate alias /%s", alias))
		}
		r.aliases[alias] = name
	}
}

// Lookup resolves user input to a Command and its argument string.
// The input should include the leading '/'. Returns (nil, "", false) if
// no match is found.
//
// Example:
//
//	cmd, args, ok := registry.Lookup("/model gpt-4")
//	// cmd = *modelCmd, args = "gpt-4", ok = true
func (r *Registry) Lookup(rawInput string) (Command, string, bool) {
	rawInput = strings.TrimSpace(rawInput)
	if !strings.HasPrefix(rawInput, "/") {
		return nil, "", false
	}

	parts := strings.SplitN(rawInput[1:], " ", 2)
	name := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	// Direct name match.
	if cmd, ok := r.commands[name]; ok {
		return cmd, args, true
	}
	// Alias match.
	if canonical, ok := r.aliases[name]; ok {
		return r.commands[canonical], args, true
	}

	return nil, "", false
}

// CommandInfos returns metadata for all commands, suitable for the
// input completer.
func (r *Registry) CommandInfos() []input.CommandInfo {
	infos := make([]input.CommandInfo, 0, len(r.order))
	for _, name := range r.order {
		cmd := r.commands[name]
		infos = append(infos, input.CommandInfo{
			Name:        cmd.Name(),
			Aliases:     cmd.Aliases(),
			Description: cmd.Description(),
		})
	}
	return infos
}

// Help returns a formatted help string listing all commands.
func (r *Registry) Help() string {
	if len(r.order) == 0 {
		return "No commands available."
	}

	var sb strings.Builder
	sb.WriteString("Available commands:\n\n")

	// Find the longest name for alignment.
	maxLen := 0
	for _, name := range r.order {
		cmd := r.commands[name]
		label := "/" + cmd.Name()
		if aliases := cmd.Aliases(); len(aliases) > 0 {
			label += " (/" + strings.Join(aliases, ", /") + ")"
		}
		if len(label) > maxLen {
			maxLen = len(label)
		}
	}

	// Sorted output.
	sorted := make([]string, len(r.order))
	copy(sorted, r.order)
	sort.Strings(sorted)

	for _, name := range sorted {
		cmd := r.commands[name]
		label := "/" + cmd.Name()
		if aliases := cmd.Aliases(); len(aliases) > 0 {
			label += " (/" + strings.Join(aliases, ", /") + ")"
		}
		fmt.Fprintf(&sb, "  %-*s  %s\n", maxLen, label, cmd.Description())
	}

	return sb.String()
}
