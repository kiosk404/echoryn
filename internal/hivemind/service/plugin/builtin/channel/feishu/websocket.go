package feishu

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/gateway"
	"github.com/kiosk404/echoryn/pkg/logger"
	lark "github.com/larksuite/oapi-sdk-go/v3"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// feishuWSClient wraps the Lark WebSocket client.
type feishuWSClient struct {
	cfg     *FeishuConfig
	client  *larkws.Client
	handler gateway.InboundHandler

	// dedup: track recently processed message IDs to avoid duplicates
	processedMu  sync.Mutex
	processedIDs map[string]time.Time

	ctx    context.Context
	cancel context.CancelFunc
}

// newFeishuWSClient creates a new WebSocket client for Feishu.
func newFeishuWSClient(cfg *FeishuConfig) *feishuWSClient {
	return &feishuWSClient{
		cfg:          cfg,
		processedIDs: make(map[string]time.Time),
	}
}

func (c *feishuWSClient) ID() string { return ChannelID }

// Start connects to Feishu via WebSocket and starts receiving events.
func (c *feishuWSClient) Start(ctx context.Context, handler gateway.InboundHandler) error {
	c.handler = handler
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Build event dispatcher
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(c.handleMessage)

	// Create WebSocket client options
	opts := []larkws.ClientOption{
		larkws.WithLogLevel(larkcore.LogLevelInfo),
		larkws.WithEventHandler(eventHandler),
		larkws.WithAutoReconnect(true),
	}

	// Set domain based on config
	domain := lark.FeishuBaseUrl
	if c.cfg.Domain == DomainLark {
		domain = lark.LarkBaseUrl
	}
	opts = append(opts, larkws.WithDomain(domain))

	c.client = larkws.NewClient(c.cfg.AppID, c.cfg.AppSecret, opts...)

	// Start WebSocket connection in background
	go func() {
		logger.Info("[Feishu] WebSocket client starting, domain=%s", c.cfg.Domain)
		err := c.client.Start(c.ctx)
		if err != nil {
			logger.Error("[Feishu] WebSocket client error: %v", err)
		}
		logger.Info("[Feishu] WebSocket client stopped")
	}()

	// Clean up expired dedup entries periodically
	go c.dedupCleaner(c.ctx)

	return nil
}

// Stop disconnects the WebSocket client.
func (c *feishuWSClient) Stop(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// handleMessage handles im.message.receive_v1 events from WebSocket.
func (c *feishuWSClient) handleMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	// Only handle text messages
	if event.Event.Message.MessageType == nil || *event.Event.Message.MessageType != "text" {
		logger.Debug("[Feishu] ignoring non-text message type: %s", ptrToStr(event.Event.Message.MessageType))
		return nil
	}

	// Dedup: skip if we've already processed this event
	eventID := ""
	if event.EventV2Base != nil && event.EventV2Base.Header != nil {
		eventID = event.EventV2Base.Header.EventID
	}
	if eventID != "" && c.isDuplicate(eventID) {
		logger.Debug("[Feishu] duplicate event, skipping: %s", eventID)
		return nil
	}

	// Parse text content from JSON
	var textContent struct {
		Text string `json:"text"`
	}
	content := ptrToStr(event.Event.Message.Content)
	if err := json.Unmarshal([]byte(content), &textContent); err != nil {
		logger.Warn("[Feishu] failed to parse message content: %v", err)
		return nil
	}

	text := strings.TrimSpace(textContent.Text)
	if text == "" {
		return nil
	}

	// Remove @bot mention prefix if present
	text = cleanBotMention(text)

	// Build InboundMessage
	chatID := ptrToStr(event.Event.Message.ChatId)
	senderID := ""
	if event.Event.Sender != nil && event.Event.Sender.SenderId != nil {
		senderID = ptrToStr(event.Event.Sender.SenderId.OpenId)
	}

	inMsg := &gateway.InboundMessage{
		ChannelID:  ChannelID,
		ChatID:     chatID,
		SenderID:   senderID,
		SenderName: senderID,
		Text:       text,
		Timestamp:  time.Now(),
		Extra: map[string]string{
			"message_id":   ptrToStr(event.Event.Message.MessageId),
			"chat_type":    ptrToStr(event.Event.Message.ChatType),
			"event_id":     eventID,
			"mention_text": content,
		},
	}

	if err := c.handler.HandleMessage(ctx, inMsg); err != nil {
		logger.Error("[Feishu] dispatch failed: chat=%s, err=%v", chatID, err)
	}

	return nil
}

// isDuplicate checks if an event has already been processed (dedup).
func (c *feishuWSClient) isDuplicate(eventID string) bool {
	c.processedMu.Lock()
	defer c.processedMu.Unlock()

	if _, exists := c.processedIDs[eventID]; exists {
		return true
	}
	c.processedIDs[eventID] = time.Now()
	return false
}

// dedupCleaner periodically removes old entries from the dedup map.
func (c *feishuWSClient) dedupCleaner(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.processedMu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for id, ts := range c.processedIDs {
				if ts.Before(cutoff) {
					delete(c.processedIDs, id)
				}
			}
			c.processedMu.Unlock()
		}
	}
}

// ptrToStr safely dereferences a string pointer.
func ptrToStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
