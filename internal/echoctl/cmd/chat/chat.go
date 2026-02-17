package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kiosk404/echoryn/internal/echoadm/utils/templates"
	"github.com/kiosk404/echoryn/internal/echoctl/cmd/util"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	"github.com/spf13/cobra"
)

var initExample = templates.Examples(`
		# Interactive chat mode (TUI)
		echoctl chat 

		# Single message mode 
		echoctl chat "Hello, introduce yourself"

		# Specify a custom session 
		echoctl chat --session=my-session "Hello, introduce yourself"

		# Connect to a specific hivemind server
		echoctl chat --server-addr=http://localhost:11780 "Hello, introduce yourself"
`)

type ChatOptions struct {
	ServerAddr string
	Session    string
	Model      string

	factory util.Factory
	genericclioptions.IOStreams
}

func NewCmdInfo(f util.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := NewChatOptions(f, ioStreams)

	cmd := &cobra.Command{
		Use:                   "chat [message]",
		DisableFlagsInUseLine: true,
		Aliases:               []string{},
		Short:                 "Chat with the Echoryn",
		Long: `
		Start a conversation with the Echoryn AI Agent through the hivemind server.

		When invoked without arguments, open an interactive TUI chat interface.
		When invoked with a message argument, send the message to the server and print the response.
		`,
		Example: initExample,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Run(cmd.Context(), args))
			util.CheckErr(o.Complete(args))
		},
		SuggestFor: []string{},
	}

	cmd.Flags().StringVar(&o.ServerAddr, "server", o.ServerAddr, "Hivemind HTTP Server Address (default: http://localhost:11789)")
	cmd.Flags().StringVar(&o.Session, "session", o.Session, "Session ID for the conversation")
	cmd.Flags().StringVar(&o.Model, "model", o.Model, "Model to use for the conversation (default: Echoryn)")

	return cmd
}

func NewChatOptions(f util.Factory, ioStreams genericclioptions.IOStreams) *ChatOptions {
	return &ChatOptions{
		factory:    f,
		IOStreams:  ioStreams,
		ServerAddr: "http://localhost:11789",
		Session:    "",
		Model:      "Echoryn",
	}
}

func (o *ChatOptions) Complete(args []string) error {
	if o.Session == "" {
		o.Session = fmt.Sprintf("echo-%s-%s", o.Model, time.Now().UnixNano())
	}
	// Ensure server address has schema
	if !strings.HasPrefix(o.ServerAddr, "http://") && !strings.HasPrefix(o.ServerAddr, "https://") {
		o.ServerAddr = "http://" + o.ServerAddr
	}
	return nil
}

func (o *ChatOptions) Run(ctx context.Context, args []string) error {
	client := NewHivemindClient(o.ServerAddr, o.Session, o.Model, o.factory.HTTPClient())

	if len(args) > 0 {
		// Single message mode : send and print response
		message := strings.Join(args, " ")
		return RunOnce(client, message, func(delta string) {
			fmt.Fprint(o.Out, delta)
		})
	}

	return RunTUI(client)
}
