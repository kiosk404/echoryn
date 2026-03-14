// Package messagebus provides the message routing layer for SubAgent inter-communication.
// It routes messages between SubAgent sessions, supporting both point-to-point and broadcast.
//
// Architecture:
//
//	MessageBus (routing layer)
//	  ├── Resolves From/To/TeamID to target sessions
//	  ├── Delegates actual delivery to SessionController (existing)
//	  └── Fires hooks for transcript persistence (TranscriptPlugin)
//
// Key design decisions:
//   - MessageBus is the UPSTREAM caller of SessionController, not a replacement
//   - All messages route through Hivemind center for audit and visibility
//   - Mailbox only holds inbox channel (reuses SessionController's SteerChannel)
package messagebus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// MessageType classifies the kind of inter-agent message.
type MessageType string

const (
	MessageTypeChat     MessageType = "chat"     // General conversation
	MessageTypeTask     MessageType = "task"     // Task assignment
	MessageTypeResult   MessageType = "result"   // Task result
	MessageTypeSteer    MessageType = "steer"    // Redirect/interrupt
	MessageTypeStatus   MessageType = "status"   // Status update
	MessageTypeShutdown MessageType = "shutdown" // Shutdown request
)

// MessagePriority controls delivery ordering.
type MessagePriority int

const (
	MessagePriorityNormal MessagePriority = 0
	MessagePriorityHigh   MessagePriority = 1
	MessagePriorityUrgent MessagePriority = 2
)

// Message is the unit of inter-agent communication.
type Message struct {
	// ID is the unique message identifier.
	ID string `json:"id"`

	// From is the sender's session ID.
	From string `json:"from"`

	// To is the recipient's session ID (empty for broadcast).
	To string `json:"to,omitempty"`

	// TeamID is the team this message belongs to.
	TeamID string `json:"team_id"`

	// Type classifies the message.
	Type MessageType `json:"type"`

	// Content is the message payload.
	Content string `json:"content"`

	// Priority controls delivery ordering.
	Priority MessagePriority `json:"priority"`

	// CreatedAt is when the message was created.
	CreatedAt time.Time `json:"created_at"`
}

// MessageHandler is a callback for topic-based subscriptions.
type MessageHandler func(ctx context.Context, msg *Message) error

// Mailbox is the inbox for a SubAgent session.
// It only holds an inbox channel — SteerChannel is managed by SessionController.
type Mailbox struct {
	SessionID string
	inbox     chan *Message
}

// NewMailbox creates a mailbox with the given buffer size.
func NewMailbox(sessionID string, bufferSize int) *Mailbox {
	return &Mailbox{
		SessionID: sessionID,
		inbox:     make(chan *Message, bufferSize),
	}
}

// Receive returns the inbox channel for consuming messages.
func (m *Mailbox) Receive() <-chan *Message {
	return m.inbox
}

// --- MessageBus Interface ---

// MessageBus manages inter-SubAgent message routing.
// It resolves recipients, delivers messages via SessionController, and fires hooks.
type MessageBus interface {
	// RegisterMailbox creates a mailbox for a session.
	RegisterMailbox(sessionID string) *Mailbox

	// UnregisterMailbox removes a session's mailbox.
	UnregisterMailbox(sessionID string)

	// Send delivers a point-to-point message.
	Send(ctx context.Context, msg *Message) error

	// Broadcast sends a message to all members of a team.
	Broadcast(ctx context.Context, teamID string, msg *Message) error

	// Subscribe registers a topic handler for a session.
	Subscribe(sessionID string, topic string, handler MessageHandler)

	// Unsubscribe removes a topic handler for a session.
	Unsubscribe(sessionID string, topic string)
}

// TeamMemberResolver resolves team membership for broadcast routing.
type TeamMemberResolver interface {
	// GetTeamMemberSessionIDs returns all session IDs for a given team.
	GetTeamMemberSessionIDs(ctx context.Context, teamID string) ([]string, error)
}

// HookEvent identifies a message bus lifecycle event.
type HookEvent string

const (
	HookMessageSent      HookEvent = "message_sent"
	HookMessageBroadcast HookEvent = "message_broadcast"
)

// HookHandler is a callback for message bus hooks.
type HookHandler func(ctx context.Context, msg *Message) error

// ResolverSetter allows setting the TeamMemberResolver after MessageBus creation.
// This breaks the circular dependency: MessageBus needs Orchestrator as resolver,
// but Orchestrator needs MessageBus.
type ResolverSetter interface {
	SetTeamMemberResolver(resolver TeamMemberResolver)
}

// HookRegistrar allows external components to register hooks on the MessageBus.
type HookRegistrar interface {
	RegisterHook(event HookEvent, handler HookHandler)
}

// --- Default Implementation ---

// defaultMessageBus is the default in-memory implementation of MessageBus.
type defaultMessageBus struct {
	mu             sync.RWMutex
	mailboxes      map[string]*Mailbox
	subscriptions  map[string]map[string]MessageHandler // sessionID → topic → handler
	teamResolver   TeamMemberResolver
	hooks          map[HookEvent][]HookHandler
	defaultBufSize int
}

// NewMessageBus creates a new MessageBus with the given team member resolver.
// If resolver is nil, it must be set later via SetTeamMemberResolver before Broadcast is called.
func NewMessageBus(teamResolver TeamMemberResolver) MessageBus {
	return &defaultMessageBus{
		mailboxes:      make(map[string]*Mailbox),
		subscriptions:  make(map[string]map[string]MessageHandler),
		teamResolver:   teamResolver,
		hooks:          make(map[HookEvent][]HookHandler),
		defaultBufSize: 100,
	}
}

// SetTeamMemberResolver implements ResolverSetter.
func (b *defaultMessageBus) SetTeamMemberResolver(resolver TeamMemberResolver) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.teamResolver = resolver
}

var _ ResolverSetter = (*defaultMessageBus)(nil)
var _ HookRegistrar = (*defaultMessageBus)(nil)

// RegisterHook adds a hook handler for a given event.
func (b *defaultMessageBus) RegisterHook(event HookEvent, handler HookHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hooks[event] = append(b.hooks[event], handler)
}

func (b *defaultMessageBus) RegisterMailbox(sessionID string) *Mailbox {
	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, ok := b.mailboxes[sessionID]; ok {
		return existing
	}

	mailbox := NewMailbox(sessionID, b.defaultBufSize)
	b.mailboxes[sessionID] = mailbox
	logger.Info("[MessageBus] registered mailbox: session=%s", sessionID)
	return mailbox
}

func (b *defaultMessageBus) UnregisterMailbox(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if mailbox, ok := b.mailboxes[sessionID]; ok {
		close(mailbox.inbox)
		delete(b.mailboxes, sessionID)
		delete(b.subscriptions, sessionID)
		logger.Info("[MessageBus] unregistered mailbox: session=%s", sessionID)
	}
}

func (b *defaultMessageBus) Send(ctx context.Context, msg *Message) error {
	if msg.To == "" {
		return fmt.Errorf("message recipient (To) is required for Send")
	}
	if msg.ID == "" {
		msg.ID = generateMessageID()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	b.mu.RLock()
	mailbox, ok := b.mailboxes[msg.To]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("recipient session %s not registered", msg.To)
	}

	// Non-blocking delivery to mailbox.
	select {
	case mailbox.inbox <- msg:
		logger.Debug("[MessageBus] delivered message: from=%s, to=%s, type=%s", msg.From, msg.To, msg.Type)
	default:
		logger.Warn("[MessageBus] mailbox full for session=%s, message dropped", msg.To)
		return fmt.Errorf("mailbox full for session %s", msg.To)
	}

	// Fire hooks asynchronously.
	go b.fireHooks(ctx, HookMessageSent, msg)

	return nil
}

func (b *defaultMessageBus) Broadcast(ctx context.Context, teamID string, msg *Message) error {
	if teamID == "" {
		return fmt.Errorf("team ID is required for Broadcast")
	}
	if msg.ID == "" {
		msg.ID = generateMessageID()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	// Resolve team member session IDs.
	if b.teamResolver == nil {
		return fmt.Errorf("team member resolver not configured; call SetTeamMemberResolver first")
	}
	sessionIDs, err := b.teamResolver.GetTeamMemberSessionIDs(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to resolve team members: %w", err)
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	var deliveryErrors []error
	for _, sessionID := range sessionIDs {
		// Skip sender.
		if sessionID == msg.From {
			continue
		}

		mailbox, ok := b.mailboxes[sessionID]
		if !ok {
			logger.Warn("[MessageBus] broadcast: session %s not registered, skipping", sessionID)
			continue
		}

		select {
		case mailbox.inbox <- msg:
			// Delivered successfully.
		default:
			deliveryErrors = append(deliveryErrors, fmt.Errorf("mailbox full for session %s", sessionID))
		}
	}

	// Fire hooks asynchronously.
	go b.fireHooks(ctx, HookMessageBroadcast, msg)

	if len(deliveryErrors) > 0 {
		logger.Warn("[MessageBus] broadcast partial failure: %d/%d deliveries failed", len(deliveryErrors), len(sessionIDs))
	}

	return nil
}

func (b *defaultMessageBus) Subscribe(sessionID string, topic string, handler MessageHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subscriptions[sessionID] == nil {
		b.subscriptions[sessionID] = make(map[string]MessageHandler)
	}
	b.subscriptions[sessionID][topic] = handler
}

func (b *defaultMessageBus) Unsubscribe(sessionID string, topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, ok := b.subscriptions[sessionID]; ok {
		delete(subs, topic)
	}
}

// fireHooks invokes all registered hooks for the given event.
func (b *defaultMessageBus) fireHooks(ctx context.Context, event HookEvent, msg *Message) {
	b.mu.RLock()
	handlers := b.hooks[event]
	b.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx, msg); err != nil {
			logger.Warn("[MessageBus] hook %s error: %v", event, err)
		}
	}
}

// --- ID Generation ---

var (
	msgCounter   uint64
	msgCounterMu sync.Mutex
)

func generateMessageID() string {
	msgCounterMu.Lock()
	defer msgCounterMu.Unlock()
	msgCounter++
	return fmt.Sprintf("msg-%d-%d", time.Now().UnixNano(), msgCounter)
}
