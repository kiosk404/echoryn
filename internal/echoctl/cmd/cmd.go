package cmd

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kiosk404/echoryn/internal/echoadm/types"
	"github.com/kiosk404/echoryn/internal/echoadm/utils/templates"
	"github.com/kiosk404/echoryn/internal/echoctl/cmd/chat"
	cmdutil "github.com/kiosk404/echoryn/internal/echoctl/cmd/util"
	genericapiserver "github.com/kiosk404/echoryn/internal/pkg/server"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	"github.com/kiosk404/echoryn/pkg/utils/cliflag"
	"github.com/kiosk404/echoryn/pkg/version/verflag"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewDefaultEchoCtlCommand creates the `echoctl` command with default arguments.
func NewDefaultEchoCtlCommand() *cobra.Command {
	return NewEchoCtlCommand(os.Stdin, os.Stdout, os.Stderr)
}

func NewEchoCtlCommand(in io.Reader, out, err io.Writer) *cobra.Command {
	// Parent command to which all subcommands are added.
	cmds := &cobra.Command{
		Use:   "echoctl",
		Short: "echoctl manages golem nodes in the echoryn realm",
		Long: templates.LongDesc(fmt.Sprintf(`%s
		echoctl is the CLI tool for managing golem nodes in the echoryn realm.

		It allows you to jion a node to a hivemind realm using a secret token,
		initialize the local node environment, and run pre-flight checks to verify that
		the node is ready to join the realm's eligibility requirements.
		Find more information at:
			https://github.com/kiosk404/echoryn/blob/master/docs/guide/en-US/cmd/echoctl/echoctl.md`, Banner())),
		Run: runHelp,
		// Hook before and after Run initialize and write profiles to disk,
		// respectively.
		PersistentPreRunE: func(*cobra.Command, []string) error {
			return initProfiling()
		},
		PersistentPostRunE: func(*cobra.Command, []string) error {
			return flushProfiling()
		},
	}
	flags := cmds.PersistentFlags()
	flags.SetNormalizeFunc(cliflag.WarnWordSepNormalizeFunc) // Warn for "_" flags

	// Normalize all flags that are coming from other packages or pre-configurations
	flags.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)

	addProfilingFlags(flags)
	addGlobalFlags(flags)

	_ = viper.BindPFlags(cmds.PersistentFlags())
	cobra.OnInitialize(func() {
		genericapiserver.LoadConfig(viper.GetString(types.FlagEchorynConfig), "echoctl")
	})
	cmds.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	// From this point and forward we get warnings on flags that contain "_" separators
	cmds.SetGlobalNormalizationFunc(cliflag.WarnWordSepNormalizeFunc)

	ioStreams := genericclioptions.IOStreams{In: in, Out: out, ErrOut: err}
	f := cmdutil.NewDefaultFactory()

	groups := templates.CommandGroups{
		{
			Message: "Basic Commands:",
			Commands: []*cobra.Command{
				chat.NewCmdInfo(f, ioStreams),
			},
		},
	}
	groups.Add(cmds)

	filters := []string{"options"}
	templates.ActsAsRootCommand(cmds, filters, groups...)

	verflag.AddFlags(cmds.PersistentFlags())

	return cmds
}

func runHelp(cmd *cobra.Command, args []string) {
	_ = cmd.Help()
}
