package token

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	cmdutil "github.com/kiosk404/echoryn/internal/echoctl/cmd/util"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"
)

// ListOptions holds options for the token list command.
type ListOptions struct {
	AdminToken string

	factory cmdutil.Factory
	genericclioptions.IOStreams
}

// NewCmdTokenList returns a cobra command for listing bootstrap tokens.
func NewCmdTokenList(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := &ListOptions{
		factory:   f,
		IOStreams: ioStreams,
	}

	c := &cobra.Command{
		Use:   "list",
		Short: "List all bootstrap tokens",
		Long:  "List all bootstrap tokens managed by the Hivemind control plane.",
		Run: func(c *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(c.Context()))
		},
	}

	c.Flags().StringVar(&o.AdminToken, "admin-token", o.AdminToken, "Admin token for authentication "+
		"(reads from credentials file if not specified)")

	return c
}

func (o *ListOptions) Validate() error {
	if o.AdminToken == "" {
		o.AdminToken = readAdminTokenFromFile()
	}
	if o.AdminToken == "" {
		return fmt.Errorf("--admin-token is required")
	}
	return nil
}

func (o *ListOptions) Run(ctx context.Context) error {
	addr := o.factory.HivemindAddr()
	client, conn, err := o.factory.AdminClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+o.AdminToken)
	resp, err := client.ListTokens(ctx, &pb.ListTokensRequest{})
	if err != nil {
		return fmt.Errorf("list tokens failed: %w", err)
	}

	if len(resp.Tokens) == 0 {
		fmt.Fprintln(o.Out, "No bootstrap tokens found.")
		return nil
	}

	w := tabwriter.NewWriter(o.Out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TOKEN ID\tEXPIRES\tUSAGES\tMAX USAGES\tDESCRIPTION")
	for _, t := range resp.Tokens {
		expires := "<never>"
		if t.ExpiresAt != nil {
			expires = t.ExpiresAt.AsTime().Format(time.RFC3339)
		}
		maxUsages := "unlimited"
		if t.MaxUsages > 0 {
			maxUsages = fmt.Sprintf("%d", t.MaxUsages)
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			t.Id, expires, t.Usages, maxUsages, t.Description)
	}
	w.Flush()

	return nil
}
