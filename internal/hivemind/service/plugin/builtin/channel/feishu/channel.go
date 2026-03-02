package feishu

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/gateway"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// ChannelID is the unique identifier for the Feishu channel.
	ChannelID = "feishu"

	// feishuSendMessageURL is the Feishu Bot API endpoint for sending messages.
	feishuSendMessageURL = "https://open.feishu.cn/open-apis/im/v1/messages"

	// feishuTenantTokenURL is the Feishu API endpoint for obtaining tenant access tokens.
	feishuTenantTokenURL = "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
)

// feishuChannel implements gateway.Channel for the Feishu (Lark) platform.
//
// It starts an HTTP webhook server to receive Feishu Bot Event Callbacks,
// normalizes them into InboundMessage, and forwards them to the dispatcher.
//
// Outbound messages are sent via the Feishu IM API (interactive card or text).
type feishuChannel struct {
	cfg     *FeishuConfig
	server  *http.Server
	handler gateway.InboundHandler

	// tenantToken cache
	mu           sync.RWMutex
	tenantToken  string
	tokenExpires time.Time

	// dedup: track recently processed message IDs to avoid duplicates
	processedMu  sync.Mutex
	processedIDs map[string]time.Time
}

// newFeishuChannel creates a new Feishu channel.
func newFeishuChannel(cfg *FeishuConfig) *feishuChannel {
	return &feishuChannel{
		cfg:          cfg,
		processedIDs: make(map[string]time.Time),
	}
}

func (c *feishuChannel) ID() string { return ChannelID }

func (c *feishuChannel) Start(ctx context.Context, handler gateway.InboundHandler) error {
	c.handler = handler

	mux := http.NewServeMux()
	mux.HandleFunc(c.cfg.WebhookPath, c.handleWebhook)

	c.server = &http.Server{
		Addr:    c.cfg.ListenAddr,
		Handler: mux,
	}

	go func() {
		logger.Info("[Feishu] webhook server listening on %s%s", c.cfg.ListenAddr, c.cfg.WebhookPath)
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("[Feishu] webhook server error: %v", err)
		}
	}()

	// Clean up expired dedup entries periodically.
	go c.dedupCleaner(ctx)

	return nil
}

func (c *feishuChannel) Stop(ctx context.Context) error {
	if c.server != nil {
		return c.server.Shutdown(ctx)
	}
	return nil
}

// handleWebhook processes Feishu Bot Event Callback HTTP requests.
//
// Feishu sends two types of requests:
// 1. URL verification challenge (type: "url_verification")
// 2. Event callbacks (type: "event_callback") containing messages
func (c *feishuChannel) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse the outer envelope.
	var envelope feishuEventEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Handle URL verification challenge.
	if envelope.Type == "url_verification" {
		c.handleChallenge(w, &envelope)
		return
	}

	// Verify token if configured.
	if c.cfg.VerificationToken != "" && envelope.Token != c.cfg.VerificationToken {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Respond immediately to avoid Feishu retry.
	w.WriteHeader(http.StatusOK)

	// Process event asynchronously.
	go c.processEvent(r.Context(), body)
}

// handleChallenge responds to Feishu's URL verification request.
func (c *feishuChannel) handleChallenge(w http.ResponseWriter, envelope *feishuEventEnvelope) {
	resp := map[string]string{"challenge": envelope.Challenge}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// processEvent parses and dispatches a Feishu event callback.
func (c *feishuChannel) processEvent(ctx context.Context, body []byte) {
	// Try v2 event format first (recommended by Feishu).
	var v2Event feishuEventV2
	if err := json.Unmarshal(body, &v2Event); err != nil {
		logger.Warn("[Feishu] failed to parse event: %v", err)
		return
	}

	// Only handle im.message.receive_v1 events.
	if v2Event.Header.EventType != "im.message.receive_v1" {
		logger.Debug("[Feishu] ignoring event type: %s", v2Event.Header.EventType)
		return
	}

	// Dedup: skip if we've already processed this event.
	eventID := v2Event.Header.EventID
	if eventID != "" && c.isDuplicate(eventID) {
		logger.Debug("[Feishu] duplicate event, skipping: %s", eventID)
		return
	}

	// Extract message content.
	msgEvent := v2Event.Event
	if msgEvent.Message.MessageType != "text" {
		logger.Debug("[Feishu] ignoring non-text message type: %s", msgEvent.Message.MessageType)
		return
	}

	// Parse text content from JSON.
	var textContent struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(msgEvent.Message.Content), &textContent); err != nil {
		logger.Warn("[Feishu] failed to parse message content: %v", err)
		return
	}

	text := strings.TrimSpace(textContent.Text)
	if text == "" {
		return
	}

	// Remove @bot mention prefix if present.
	text = cleanBotMention(text)

	// Build InboundMessage.
	inMsg := &gateway.InboundMessage{
		ChannelID:  ChannelID,
		ChatID:     msgEvent.Message.ChatID,
		SenderID:   msgEvent.Sender.SenderID.OpenID,
		SenderName: msgEvent.Sender.SenderID.OpenID, // Will be resolved later if needed.
		Text:       text,
		Timestamp:  time.Now(),
		Extra: map[string]string{
			"message_id":   msgEvent.Message.MessageID,
			"chat_type":    msgEvent.Message.ChatType,
			"event_id":     eventID,
			"mention_text": msgEvent.Message.Content,
		},
	}

	if err := c.handler.HandleMessage(ctx, inMsg); err != nil {
		logger.Error("[Feishu] dispatch failed: chat=%s, err=%v", msgEvent.Message.ChatID, err)
	}
}

// isDuplicate checks if an event has already been processed (dedup).
func (c *feishuChannel) isDuplicate(eventID string) bool {
	c.processedMu.Lock()
	defer c.processedMu.Unlock()

	if _, exists := c.processedIDs[eventID]; exists {
		return true
	}
	c.processedIDs[eventID] = time.Now()
	return false
}

// dedupCleaner periodically removes old entries from the dedup map.
func (c *feishuChannel) dedupCleaner(ctx context.Context) {
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

// cleanBotMention removes @bot mention prefixes from text.
func cleanBotMention(text string) string {
	// Feishu @mentions appear as @_user_1 in text content.
	// Remove common patterns.
	for _, prefix := range []string{"@_user_1 ", "@_user_1"} {
		text = strings.TrimPrefix(text, prefix)
	}
	return strings.TrimSpace(text)
}

// --- Feishu Event Structures ---

type feishuEventEnvelope struct {
	Type      string `json:"type"`
	Token     string `json:"token"`
	Challenge string `json:"challenge"`
}

type feishuEventV2 struct {
	Schema string         `json:"schema"`
	Header feishuV2Header `json:"header"`
	Event  feishuMsgEvent `json:"event"`
}

type feishuV2Header struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	TenantKey string `json:"tenant_key"`
	AppID     string `json:"app_id"`
}

type feishuMsgEvent struct {
	Sender  feishuSender  `json:"sender"`
	Message feishuMessage `json:"message"`
}

type feishuSender struct {
	SenderID feishuSenderID `json:"sender_id"`
}

type feishuSenderID struct {
	OpenID  string `json:"open_id"`
	UserID  string `json:"user_id"`
	UnionID string `json:"union_id"`
}

type feishuMessage struct {
	MessageID   string `json:"message_id"`
	ChatID      string `json:"chat_id"`
	ChatType    string `json:"chat_type"`
	MessageType string `json:"message_type"`
	Content     string `json:"content"`
}

// --- Outbound: Feishu Message API ---

// feishuOutbound implements gateway.OutboundAdapter for sending messages via Feishu API.
type feishuOutbound struct {
	channel *feishuChannel
}

func newFeishuOutbound(ch *feishuChannel) *feishuOutbound {
	return &feishuOutbound{channel: ch}
}

func (o *feishuOutbound) SendText(ctx context.Context, chatID string, text string, opts *gateway.SendOptions) error {
	content := map[string]string{"text": text}
	contentJSON, _ := json.Marshal(content)

	return o.sendMessage(ctx, chatID, "text", string(contentJSON), opts)
}

func (o *feishuOutbound) SendMarkdown(ctx context.Context, chatID string, markdown string, opts *gateway.SendOptions) error {
	// Feishu doesn't natively support Markdown in text messages.
	// Use interactive card for rich formatting.
	card := buildMarkdownCard(markdown)
	cardJSON, _ := json.Marshal(card)

	return o.sendMessage(ctx, chatID, "interactive", string(cardJSON), opts)
}

// sendMessage sends a message via the Feishu IM API.
func (o *feishuOutbound) sendMessage(ctx context.Context, chatID, msgType, content string, opts *gateway.SendOptions) error {
	token, err := o.channel.getTenantToken(ctx)
	if err != nil {
		return fmt.Errorf("get tenant token: %w", err)
	}

	payload := map[string]string{
		"receive_id": chatID,
		"msg_type":   msgType,
		"content":    content,
	}

	// If replying to a specific message.
	if opts != nil && opts.ReplyTo != "" {
		payload["reply_in_thread"] = "false"
	}

	payloadJSON, _ := json.Marshal(payload)

	url := feishuSendMessageURL + "?receive_id_type=chat_id"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("feishu API error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// getTenantToken obtains or refreshes the Feishu tenant access token.
func (c *feishuChannel) getTenantToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.tenantToken != "" && time.Now().Before(c.tokenExpires) {
		token := c.tenantToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock.
	if c.tenantToken != "" && time.Now().Before(c.tokenExpires) {
		return c.tenantToken, nil
	}

	payload, _ := json.Marshal(map[string]string{
		"app_id":     c.cfg.AppID,
		"app_secret": c.cfg.AppSecret,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, feishuTenantTokenURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch tenant token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu token error: code=%d, msg=%s", result.Code, result.Msg)
	}

	c.tenantToken = result.TenantAccessToken
	// Refresh 5 minutes before expiry.
	c.tokenExpires = time.Now().Add(time.Duration(result.Expire-300) * time.Second)

	return c.tenantToken, nil
}

// buildMarkdownCard builds a Feishu interactive card with markdown content.
func buildMarkdownCard(markdown string) map[string]interface{} {
	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"elements": []map[string]interface{}{
			{
				"tag":     "markdown",
				"content": markdown,
			},
		},
	}
}

// verifySignature verifies the Feishu event callback signature (optional).
func verifySignature(timestamp, nonce, encryptKey, body, signature string) bool {
	content := timestamp + nonce + encryptKey + body
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash) == signature
}

var _ gateway.Channel = (*feishuChannel)(nil)
var _ gateway.OutboundAdapter = (*feishuOutbound)(nil)
