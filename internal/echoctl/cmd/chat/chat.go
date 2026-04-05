package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kiosk404/echoryn/internal/echoctl/cmd/util"
	"github.com/kiosk404/echoryn/internal/echoctl/utils/templates"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	chatui "github.com/kiosk404/echoryn/pkg/cli/tui"
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
	Resume     string
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
			util.CheckErr(o.Complete(args))
			util.CheckErr(o.Run(cmd.Context(), args))
		},
		SuggestFor: []string{},
	}

	cmd.Flags().StringVar(&o.ServerAddr, "server", o.ServerAddr, "Hivemind HTTP Server Address (default: http://localhost:11789)")
	cmd.Flags().StringVar(&o.Session, "session", o.Session, "Session ID for the conversation")
	cmd.Flags().StringVar(&o.Resume, "resume", "", "Resume a previous session by ID (printed on exit)")
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
	if o.Resume != "" {
		o.Session = o.Resume
	}
	if o.Session == "" {
		o.Session = fmt.Sprintf("echo-%s-%d", o.Model, time.Now().UnixNano())
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
		// Single message mode : send and print response.
		message := strings.Join(args, " ")
		return RunOnce(client, message, func(delta string) {
			fmt.Fprint(o.Out, delta)
		})
	}

	// Interactive TUI mode.
	adapter := newClientAdapter(client)
	teamAPI := NewTeamHTTPClient(o.ServerAddr, o.Session, client.HTTPClient)
	teamSub := NewTeamHTTPSubscriber(o.ServerAddr, o.ServerAddr, client.HTTPClient)
	ui := chatui.New(adapter,
		chatui.WithProgramName("echoctl"),
		chatui.WithTeamAPI(teamAPI),
		chatui.WithTeamEventSubscriber(teamSub))

	return ui.Run(ctx)
}

// newClientAdapter bridges the concrete HivemindClient and the TUI's
// Client interface, converting between the two ChatMessage types.
func newClientAdapter(c *HivemindClient) *chatui.ClientAdapter {
	return &chatui.ClientAdapter{
		ChatStreamFn: func(
			ctx context.Context,
			msgs []chatui.ChatMessage,
			cb chatui.StreamCallback,
			toolCb chatui.ToolCallCallback,
		) (string, error) {
			// Convert tui.ChatMessage → chat.ChatMessage.
			apiMsgs := make([]ChatMessage, len(msgs))
			for i, m := range msgs {
				apiMsgs[i] = ChatMessage{Role: m.Role, Content: m.Content}
			}
			return c.ChatStream(ctx, apiMsgs, StreamCallback(cb), ToolCallCallback(toolCb))
		},
		ModelName: c.Model,
		ServerURL: c.BaseURL,
		Session:   c.SessionKey,
	}
}
