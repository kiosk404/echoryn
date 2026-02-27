package init

import (
	cmdutil "github.com/kiosk404/echoryn/internal/echoadm/cmd/util"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	"github.com/spf13/cobra"
)

// NewCmdInit returns the 'echoryn init' command with hivemind and golem subcommands.
func NewCmdInit(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize an echoryn node (Hivemind control plane or Golem worker)",
		Long: `Initialize an echoryn node. Use one of the subcommands to specify the role:

  echoadm init hivemind   Initialize a Hivemind control plane node
  echoadm init golem      Initialize a Golem worker node`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdInitHivemind(f, ioStreams))
	cmd.AddCommand(NewCmdInitGolem(f, ioStreams))

	return cmd
}
