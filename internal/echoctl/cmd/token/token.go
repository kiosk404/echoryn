package token

import (
	cmdutil "github.com/kiosk404/echoryn/internal/echoctl/cmd/util"
	"github.com/kiosk404/echoryn/internal/echoctl/utils/templates"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	"github.com/spf13/cobra"
)

// NewCmdToken returns the `echoctl token` command group
func NewCmdToken(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage bootstrap tokens for Golem node joining",
		Long: templates.LongDesc(`
		Manage bootstrap tokens used by Golem worker nodes to register
		with the Hivemind control plane.

		Bootstrap tokens are short-lived, limit-use secrets following
		the pattern <6-char-id>.<16-char-secret.>
		`),
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdTokenCreate(f, ioStreams))
	cmd.AddCommand(NewCmdTokenDelete(f, ioStreams))

	return cmd
}
