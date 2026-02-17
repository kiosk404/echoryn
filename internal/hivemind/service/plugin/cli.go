package plugin

import (
	"github.com/spf13/cobra"
)

// CLIRegistrar is the interface for plugins that register CLI commands.
// This corresponds to openclaw's registerCli() capability.
type CLIRegistrar interface {
	// RegisterCommands adds subcommands to the given parent command
	RegisterCommands(parent *cobra.Command)
}

// CLIProvider is an optional plugin interface for plugins that provide CLI commands.
// This corresponds to openclaw's registerCli() capability.
type CLIProvider interface {
	Plugin

	// CLIRegistrars returns the CLIRegistrars provided by this plugin.
	CLIRegistrars() []CLIRegistrar
}
