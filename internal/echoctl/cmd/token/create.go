package token

import (
	"context"
	"fmt"
	"time"

	cmdutil "github.com/kiosk404/echoryn/internal/echoctl/cmd/util"
	"github.com/kiosk404/echoryn/internal/echoctl/utils/templates"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/durationpb"
)

var createExample = templates.Examples(`
		# Create a token with default TTL (24h) and unlimited usages
		# (reads admin token from ~/.echoryn/credentials/admin_token if not specified)
		echoctl token create --admin-token=ecr-admin.xxxxxxxx

		# Create a token with custom TTL and max usages
		echoctl token create --ttl=2h --max-usages=10 --description="CI nodes" --admin-token=ecr-admin.xxxxxxxx
`)

// CreateOptions holds options for the token create command.
type CreateOptions struct {
	AdminToken  string
	TTL         string
	MaxUsages   int32
	Description string

	factory cmdutil.Factory
	genericclioptions.IOStreams
}

// NewCmdTokenCreate returns a cobra command for creating bootstrap tokens.
func NewCmdTokenCreate(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := &CreateOptions{
		TTL:       "24h",
		MaxUsages: 0,
		factory:   f,
		IOStreams: ioStreams,
	}

	c := &cobra.Command{
		Use:     "create",
		Short:   "Create a new bootstrap token",
		Long:    "Create a new bootstrap token for Golem nodes to join the Hivemind cluster.",
		Example: createExample,
		Run: func(c *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(c.Context()))
		},
	}

	c.Flags().StringVar(&o.AdminToken, "admin-token", o.AdminToken, "Admin token for authentication "+
		"(reads from credentials file if not specified)")
	c.Flags().StringVar(&o.TTL, "ttl", o.TTL, "Token time-to-live (e.g. 2h, 24h, 168h)")
	c.Flags().Int32Var(&o.MaxUsages, "max-usages", o.MaxUsages, "Maximum number of uses (0 = unlimited)")
	c.Flags().StringVar(&o.Description, "description", o.Description, "Human-readable description")

	return c
}

func (o *CreateOptions) Validate() error {
	if o.AdminToken == "" {
		o.AdminToken = readAdminTokenFromFile()
	}
	if o.AdminToken == "" {
		return fmt.Errorf("--admin-token is required")
	}
	if _, err := time.ParseDuration(o.TTL); err != nil {
		return fmt.Errorf("invalid --ttl: %w", err)
	}
	return nil
}

func (o *CreateOptions) Run(ctx context.Context) error {
	addr := o.factory.HivemindAddr()
	client, conn, err := o.factory.AdminClient(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	ttl, _ := time.ParseDuration(o.TTL)

	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+o.AdminToken)
	resp, err := client.CreateToken(ctx, &pb.CreateTokenRequest{
		Ttl:         durationpb.New(ttl),
		MaxUsages:   o.MaxUsages,
		Description: o.Description,
	})
	if err != nil {
		return fmt.Errorf("create token failed: %w", err)
	}

	fmt.Fprintf(o.Out, "Bootstrap token created successfully.\n")
	fmt.Fprintf(o.Out, "Token:       %s\n", resp.Token)
	if resp.TokenInfo != nil {
		fmt.Fprintf(o.Out, "Token ID:    %s\n", resp.TokenInfo.Id)
		if resp.TokenInfo.ExpiresAt != nil {
			fmt.Fprintf(o.Out, "Expires:     %s\n", resp.TokenInfo.ExpiresAt.AsTime().Format(time.RFC3339))
		}
		fmt.Fprintf(o.Out, "Max Usages:  %d\n", resp.TokenInfo.MaxUsages)
		fmt.Fprintf(o.Out, "Description: %s\n", resp.TokenInfo.Description)
	}
	fmt.Fprintf(o.Out, "\nPlease save this token — it will not be shown again.\n")
	fmt.Fprintf(o.Out, "Use it with: golem --join-token=%s\n", resp.Token)

	return nil
}
