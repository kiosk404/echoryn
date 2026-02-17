package join

import (
	"context"
	"time"

	"github.com/kiosk404/echoryn/internal/echoadm/cmd/util"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	"github.com/spf13/cobra"
)

var joinExample = `
		# Join a hivemind realm using a secret token
		echoctl jion --token=<PASSWORD>

		# Join with a custom server address
		echoctl jion --token=<PASSWORD> --hivemind-addr=127.0.0.1:11788

		# Join with a custom node name
		echoctl jion --token=<PASSWORD> --node-name=golem-1
`

type Join struct {
	Token string

	NodeName string

	Timeout time.Duration

	SkipChecks bool

	Factory util.Factory
	genericclioptions.IOStreams
}

func NewJoinOptions(f util.Factory, ioStreams genericclioptions.IOStreams) *Join {
	return &Join{
		Timeout:   30 * time.Second,
		Factory:   f,
		IOStreams: ioStreams,
	}
}

func NewCmdJoin(f util.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := NewJoinOptions(f, ioStreams)

	cmd := &cobra.Command{
		Use:                   "join",
		DisableFlagsInUseLine: true,
		Aliases:               []string{},
		Short:                 "Join this node to a hivemind using a secret token",
		Long: `Register this node as a golem worker node in a hivemind realm using a secret token
		The hivemind central controller issues a one-time secret token to each new node. Provider it via --token flag
		to authenticate and complete registration. before joining the hivemind realm. the command runs pre-flight checks
		to verify that the node is ready to join the realm's eligibility requirements.
		`,
		Example: joinExample,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run(cmd.Context(), args))
		},
		SuggestFor: []string{},
	}

	cmd.Flags().StringVar(&o.Token, "token", o.Token, "The secret token to use for joining the hivemind realm")
	cmd.Flags().StringVar(&o.NodeName, "node-name", o.NodeName, "The name of this node")
	cmd.Flags().DurationVar(&o.Timeout, "timeout", o.Timeout, "The timeout duration for the join operation")
	cmd.Flags().BoolVar(&o.SkipChecks, "skip-checks", o.SkipChecks, "Skip pre-flight checks")

	return cmd
}

func (o *Join) Run(ctx context.Context, args []string) error {

	return nil
}

func (o *Join) Validate() error {
	return nil
}
