package config

import (
	"fmt"
	"os"
	"strings"

	cmdutil "github.com/kiosk404/echoryn/internal/echoadm/cmd/util"
	admconfig "github.com/kiosk404/echoryn/internal/echoadm/config"
	"github.com/kiosk404/echoryn/internal/echoadm/globals"
	"github.com/kiosk404/echoryn/internal/echoadm/utils/templates"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	"github.com/kiosk404/echoryn/pkg/utils/json"
	"github.com/spf13/cobra"
)

// NewCmdConfig returns the 'echoadm config' command with subcommands.
func NewCmdConfig(_ cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage echoryn configuration",
		Long:  "View, modify, and validate the echoryn configuration file (~/.echoryn/config.json).",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(newCmdConfigGet(ioStreams))
	cmd.AddCommand(newCmdConfigSet(ioStreams))
	cmd.AddCommand(newCmdConfigUnset(ioStreams))
	cmd.AddCommand(newCmdConfigView(ioStreams))
	cmd.AddCommand(newCmdConfigValidate(ioStreams))

	return cmd
}

func getConfigPath() string {
	if globals.ConfigPath != "" {
		return globals.ConfigPath
	}
	return admconfig.DefaultConfigPath()
}

// --- config get ---

var getExample = templates.Examples(`
		# Get the default model provider
		echoadm config get models.default_provider

		# Get the hivemind address
		echoadm config get hivemind.address
`)

func newCmdConfigGet(ioStreams genericclioptions.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "get <key>",
		Short:   "Get a configuration value by dot-path key",
		Example: getExample,
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := runConfigGet(ioStreams, args[0]); err != nil {
				fmt.Fprintf(ioStreams.ErrOut, "error: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

func runConfigGet(ioStreams genericclioptions.IOStreams, key string) error {
	cfg, err := admconfig.Load(getConfigPath())
	if err != nil {
		return err
	}

	val, err := admconfig.GetByDotPath(cfg, key)
	if err != nil {
		return err
	}

	switch v := val.(type) {
	case string:
		fmt.Fprintln(ioStreams.Out, v)
	case map[string]interface{}:
		data, _ := json.MarshalIndent(v, "", "  ")
		fmt.Fprintln(ioStreams.Out, string(data))
	default:
		fmt.Fprintf(ioStreams.Out, "%v\n", v)
	}

	return nil
}

// --- config set ---

var setExample = templates.Examples(`
		# Set the default model provider
		echoadm config set models.default_provider openai

		# Set a boolean value
		echoadm config set plugins.enabled true

		# Set a numeric value
		echoadm config set hivemind.grpc_port 11788
`)

func newCmdConfigSet(ioStreams genericclioptions.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "set <key> <value>",
		Short:   "Set a configuration value by dot-path key",
		Example: setExample,
		Args:    cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if err := runConfigSet(ioStreams, args[0], args[1]); err != nil {
				fmt.Fprintf(ioStreams.ErrOut, "error: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

func runConfigSet(ioStreams genericclioptions.IOStreams, key, value string) error {
	configPath := getConfigPath()

	cfg, err := admconfig.Load(configPath)
	if err != nil {
		return err
	}

	if err := admconfig.SetByDotPath(cfg, key, value); err != nil {
		return err
	}

	if err := admconfig.Save(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(ioStreams.Out, "Set %s = %s\n", key, value)
	return nil
}

// --- config unset ---

var unsetExample = templates.Examples(`
		# Remove a provider configuration
		echoadm config unset models.providers.openai
`)

func newCmdConfigUnset(ioStreams genericclioptions.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "unset <key>",
		Short:   "Remove a configuration key by dot-path",
		Example: unsetExample,
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := runConfigUnset(ioStreams, args[0]); err != nil {
				fmt.Fprintf(ioStreams.ErrOut, "error: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

func runConfigUnset(ioStreams genericclioptions.IOStreams, key string) error {
	configPath := getConfigPath()

	cfg, err := admconfig.Load(configPath)
	if err != nil {
		return err
	}

	if err := admconfig.UnsetByDotPath(cfg, key); err != nil {
		return err
	}

	if err := admconfig.Save(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(ioStreams.Out, "Unset %s\n", key)
	return nil
}

// --- config view ---

func newCmdConfigView(ioStreams genericclioptions.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:   "view",
		Short: "Display the full configuration (with secrets masked)",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := runConfigView(ioStreams); err != nil {
				fmt.Fprintf(ioStreams.ErrOut, "error: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

func runConfigView(ioStreams genericclioptions.IOStreams) error {
	cfg, err := admconfig.Load(getConfigPath())
	if err != nil {
		return err
	}

	sanitized := admconfig.SanitizedView(cfg)
	data, _ := json.MarshalIndent(sanitized, "", "  ")
	fmt.Fprintln(ioStreams.Out, string(data))
	return nil
}

// --- config validate ---

func newCmdConfigValidate(ioStreams genericclioptions.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the configuration file",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := runConfigValidate(ioStreams); err != nil {
				fmt.Fprintf(ioStreams.ErrOut, "error: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

func runConfigValidate(ioStreams genericclioptions.IOStreams) error {
	configPath := getConfigPath()

	if !admconfig.Exists(configPath) {
		return fmt.Errorf("config file not found at %s. Run 'echoadm init' first", configPath)
	}

	cfg, err := admconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	issues := admconfig.Validate(cfg)

	if len(issues) == 0 {
		fmt.Fprintf(ioStreams.Out, "Configuration is valid. (%s)\n", configPath)
		return nil
	}

	fmt.Fprintf(ioStreams.Out, "Configuration issues found in %s:\n\n", configPath)
	for i, issue := range issues {
		prefix := "WARNING"
		if strings.Contains(issue, "required") {
			prefix = "ERROR"
		}
		fmt.Fprintf(ioStreams.Out, "  [%s] %d. %s\n", prefix, i+1, issue)
	}
	fmt.Fprintln(ioStreams.Out)

	return nil
}
