package tui

import (
	"context"
)

// ChatStreamFunc is the function signature of HivemindClient.ChatStream.
// It is used by [ClientAdapter] to avoid a direct import cycle between
// the chat package and the tui package.
type ChatStreamFunc func(
	ctx context.Context,
	messages []ChatMessage,
	cb StreamCallback,
	toolCb ToolCallCallback,
) (string, error)

// ClientAdapter wraps the raw function pointers from the chat package's
// HivemindClient so that they satisfy the [Client] interface.
//
// This thin adapter exists to break the import cycle:
//
//	chat → tui.Client (interface)
//	chat → tui.ClientAdapter (creates from HivemindClient)
//
// Without exposing HivemindClient's concrete type to the tui package.
type ClientAdapter struct {
	ChatStreamFn ChatStreamFunc
	ModelName    string
	ServerURL    string
	Session      string
}

// Ensure ClientAdapter implements Client at compile time.
var _ Client = (*ClientAdapter)(nil)

// ChatStream delegates to the wrapped function.
func (a *ClientAdapter) ChatStream(
	ctx context.Context,
	messages []ChatMessage,
	cb StreamCallback,
	toolCb ToolCallCallback,
) (string, error) {
	return a.ChatStreamFn(ctx, messages, cb, toolCb)
}

// Model returns the configured model name.
func (a *ClientAdapter) Model() string { return a.ModelName }

// BaseURL returns the server address.
func (a *ClientAdapter) BaseURL() string { return a.ServerURL }

// SessionKey returns the session identifier.
func (a *ClientAdapter) SessionKey() string { return a.Session }
