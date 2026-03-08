package token

import (
	"context"
	"fmt"

	cmdutil "github.com/kiosk404/echoryn/internal/echoctl/cmd/util"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"
)

// DeleteOptions holds options for the token delete command.
type DeleteOptions struct {
	AdminToken string
	TokenID    string

	factory cmdutil.Factory
	genericclioptions.IOStreams
}

// NewCmdTokenDelete returns a cobra command for deleting a bootstrap token.
func NewCmdTokenDelete(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := &DeleteOptions{
		factory:   f,
		IOStreams: ioStreams,
	}

	c := &cobra.Command{
		Use:   "delete <token-id>",
		Short: "Delete a bootstrap token",
		Long:  "Delete a bootstrap token by its 6-character ID",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			o.TokenID = args[0]
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(cmd.Context()))
		},
	}

	c.Flags().StringVar(&o.AdminToken, "admin-token", o.AdminToken, "Admin token for authentication "+
		"(reads from credentials file if not specified)")

	return c
}

func (o *DeleteOptions) Validate() error {
	if o.AdminToken == "" {
		o.AdminToken = readAdminTokenFromFile()
	}
	if o.AdminToken == "" {
		return fmt.Errorf("--admin-token is required")
	}
	if o.TokenID == "" {
		return fmt.Errorf("token-id argument is required")
	}
	return nil
}

func (o *DeleteOptions) Run(ctx context.Context) error {
	addr := o.factory.HivemindAddr()
	client, conn, err := o.factory.AdminClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+o.AdminToken)
	resp, err := client.DeleteToken(ctx, &pb.DeleteTokenRequest{
		TokenId: o.TokenID,
	})
	if err != nil {
		return fmt.Errorf("delete token failed: %w", err)
	}
	if resp.Success {
		fmt.Fprintf(o.Out, "Token %s deleted successfully.\n", o.TokenID)
	} else {
		fmt.Fprintf(o.Out, "Failed to delete token %s.\n", o.TokenID)
	}
	return nil
}
