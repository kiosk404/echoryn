package eidoctl

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kiosk404/eidolon/internal/eidoctl/cmd/info"
	"github.com/kiosk404/eidolon/internal/eidoctl/cmd/init"
	"github.com/kiosk404/eidolon/internal/eidoctl/cmd/join"
	cmdutil "github.com/kiosk404/eidolon/internal/eidoctl/cmd/util"
	genericapiserver "github.com/kiosk404/eidolon/internal/pkg/server"
	"github.com/kiosk404/eidolon/pkg/cli/genericclioptions"
	"github.com/kiosk404/eidolon/pkg/utils/cliflag"
	"github.com/kiosk404/eidolon/pkg/utils/templates"
	"github.com/kiosk404/eidolon/pkg/version/verflag"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewDefaultHivCtlCommand creates the `eidoctl` command with default arguments.
func NewDefaultHivCtlCommand() *cobra.Command {
	return NewHivCtlCommand(os.Stdin, os.Stdout, os.Stderr)
}

func NewHivCtlCommand(in io.Reader, out, err io.Writer) *cobra.Command {
	// Parent command to which all subcommands are added.
	cmds := &cobra.Command{
		Use:   "eidoctl",
		Short: "eidoctl manages golem nodes in the eidolon realm",
		Long: templates.LongDesc(fmt.Sprintf(`%s
		eidoctl is the CLI tool for managing golem nodes in the eidolon realm.

		It allows you to jion a node to a hivemind realm using a secret token,
		initialize the local node environment, and run pre-flight checks to verify that
		the node is ready to join the realm's eligibility requirements.
		Find more information at:
			https://github.com/kiosk404/eidolon/blob/master/docs/guide/en-US/cmd/eidoctl/eidoctl.md`, Banner())),
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
		// genericapiserver.LoadConfig(viper.GetString(genericclioptions.FlagIAMConfig), "eidoctl")
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
				init.NewCmdInit(f, ioStreams),
				join.NewCmdJoin(f, ioStreams),
			},
		},
		{
			Message: "Diagnostic Commands:",
			Commands: []*cobra.Command{
				info.NewCmdInfo(f, ioStreams),
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
