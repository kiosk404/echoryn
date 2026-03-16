// Package gateway implements the IM platform gateway layer for Echoryn.
//
// This is the Echoryn equivalent of OpenClaw's Gateway + Channel system,
// providing a unified abstraction for connecting external IM platforms
// (Feishu, Telegram, Discord, etc.) to the Agent runtime.
//
// Architecture:
//
//	IM Platform SDK → Channel.Start() → InboundHandler.HandleMessage()
//	                                        ↓
//	                                   Dispatcher → AgentService.Run()
//	                                        ↓
//	                              Deliverer → OutboundAdapter.Send*()
//	                                        ↓
//	                                  IM Platform API
package gateway

import (
	"context"
	"time"
)

// Channel is the core interface that each IM platform must implement.
// It encapsulates the platform-specific logic for receiving and sending messages.
//
// This is the Go equivalent of OpenClaw's ChannelPlugin, simplified to
// follow Go's small-interface philosophy (3 methods vs 15+ adapters).
//
// Implementations: channel-feishu, channel-telegram
type Channel interface {
	// ID returns the unique identifier for this channel (e.g., "feishu", "telegram").
	ID() string

	// Start launches the channel, connecting to the IM platform and
	// listening for inbound messages. Messages are delivered via the handler.
	// Start should be non-blocking (launch goroutines internally).
	// The context is used for lifecycle control — cancellation means stop.
	Start(ctx context.Context, handler InboundHandler) error

	// Stop gracefully shuts down the channel, disconnecting from the IM platform.
	Stop(ctx context.Context) error
}

// InboundHandler is the callback interface that channels use to deliver
// received messages to the gateway dispatcher.
type InboundHandler interface {
	// HandleMessage processes an inbound message from an IM platform.
	// The dispatcher maps it to an Agent session and invokes AgentService.Run().
	HandleMessage(ctx context.Context, msg *InboundMessage) error
}

// InboundMessage is the normalized representation of a message received
// from an IM platform. Each Channel implementation converts platform-specific
// formats into this structure.
//
// This corresponds to OpenClaw's normalized message format after the
// normalize/ layer processes platform-specific events.
type InboundMessage struct {
	// ChannelID is the source channel identifier (e.g., "feishu", "telegram").
	ChannelID string `json:"channel_id"`

	// ChatID is the platform-specific conversation/group ID.
	// Used for session mapping: sessionID = "{channel_id}:{chat_id}"
	ChatID string `json:"chat_id"`

	// SenderID is the platform-specific user identifier.
	SenderID string `json:"sender_id"`

	// SenderName is the human-readable display name of the sender.
	SenderName string `json:"sender_name"`

	// Text is the plain text content of the message.
	Text string `json:"text"`

	// ReplyTo is the platform-specific message ID being replied to (optional).
	ReplyTo string `json:"reply_to,omitempty"`

	// Attachments holds any media/file attachments (optional).
	Attachments []Attachment `json:"attachments,omitempty"`

	// Timestamp is when the message was sent on the IM platform.
	Timestamp time.Time `json:"timestamp"`

	// Extra holds platform-specific extension fields.
	Extra map[string]string `json:"extra,omitempty"`
}

// Attachment represents a media or file attachment in an inbound message.
type Attachment struct {
	// Type is the attachment type (e.g., "image", "file", "audio", "video").
	Type string `json:"type"`

	// URL is the download URL for the attachment.
	URL string `json:"url,omitempty"`

	// Name is the filename (optional).
	Name string `json:"name,omitempty"`

	// MimeType is the MIME type (optional).
	MimeType string `json:"mime_type,omitempty"`
}

// OutboundAdapter is the interface for sending messages back to an IM platform.
// Each Channel implementation provides its own OutboundAdapter.
//
// This corresponds to OpenClaw's ChannelOutboundAdapter.
type OutboundAdapter interface {
	// SendText sends a plain text message to the specified chat.
	SendText(ctx context.Context, chatID string, text string, opts *SendOptions) error

	// SendMarkdown sends a Markdown-formatted message to the specified chat.
	// For platforms that don't support Markdown, it falls back to plain text.
	SendMarkdown(ctx context.Context, chatID string, markdown string, opts *SendOptions) error
}

// SendOptions holds optional parameters for outbound message delivery.
type SendOptions struct {
	// ReplyTo is the message ID to reply to (optional, platform-specific).
	ReplyTo string

	// Silent suppresses notification on the recipient's device.
	Silent bool

	// AddWorkingIndicator adds "working" indicator (e.g. emoji reaction)
	// to the sent message to show the bot is processing.
	AddWorkingIndicator bool
}

// ChannelConfig is the configuration for a single channel instance.
// Each channel type has its own specific fields, but these are common.
type ChannelConfig struct {
	// Enabled controls whether this channel is active.
	Enabled bool `json:"enabled"`

	// AgentID is the default agent to route messages to.
	// If empty, uses the gateway default agent.
	AgentID string `json:"agent_id,omitempty"`

	// Platform-specific configuration is stored in the channel's own config struct.
}
