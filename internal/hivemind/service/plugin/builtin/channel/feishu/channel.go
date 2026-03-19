package feishu

import (
	"bytes"
	"context"
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

	// feishuReactionURL is the Feishu API endpoint for message reactions.
	feishuReactionURL = "https://open.feishu.cn/open-apis/im/v1/messages"

	// larkSendMessageURL is the Lark (international) Bot API endpoint for sending messages.
	larkSendMessageURL = "https://open.larksuite.com/open-apis/im/v1/messages"

	// larkTenantTokenURL is the Lark (international) API endpoint for obtaining tenant access tokens.
	larkTenantTokenURL = "https://open.larksuite.com/open-apis/auth/v3/tenant_access_token/internal"

	// larkReactionURL is the Lark (international) API endpoint for message reactions.
	larkReactionURL = "https://open.larksuite.com/open-apis/im/v1/messages"
)

// feishuChannel implements gateway.Channel for the Feishu (Lark) platform.
//
// It supports two connection modes:
// 1. WebSocket (default): Client connects to Feishu cloud, no public IP required.
// 2. Webhook: Starts an HTTP server to receive callbacks, requires public IP.
type feishuChannel struct {
	cfg     *FeishuConfig
	handler gateway.InboundHandler

	// WebSocket client (for websocket mode)
	wsClient *feishuWSClient

	// HTTP server (for webhook mode)
	server *http.Server

	// tenantToken cache (shared across modes)
	mu           sync.RWMutex
	tenantToken  string
	tokenExpires time.Time
}

// newFeishuChannel creates a new Feishu channel.
func newFeishuChannel(cfg *FeishuConfig) *feishuChannel {
	return &feishuChannel{
		cfg: cfg,
	}
}

func (c *feishuChannel) ID() string { return ChannelID }

// Start initializes the channel based on the configured connection mode.
func (c *feishuChannel) Start(ctx context.Context, handler gateway.InboundHandler) error {
	c.handler = handler

	switch c.cfg.ConnectionMode {
	case ConnectionModeWebsocket:
		return c.startWebSocket(ctx)
	case ConnectionModeWebhook:
		return c.startWebhook(ctx)
	default:
		// Default to WebSocket
		logger.Info("[Feishu] unknown connection mode '%s', defaulting to websocket", c.cfg.ConnectionMode)
		return c.startWebSocket(ctx)
	}
}

// Stop stops the channel based on the current connection mode.
func (c *feishuChannel) Stop(ctx context.Context) error {
	if c.wsClient != nil {
		return c.wsClient.Stop(ctx)
	}
	if c.server != nil {
		return c.server.Shutdown(ctx)
	}
	return nil
}

// startWebSocket starts the WebSocket client mode.
func (c *feishuChannel) startWebSocket(ctx context.Context) error {
	c.wsClient = newFeishuWSClient(c.cfg)
	logger.Info("[Feishu] starting in WebSocket mode (domain=%s)", c.cfg.Domain)
	return c.wsClient.Start(ctx, c.handler)
}

// startWebhook starts the HTTP webhook server mode.
func (c *feishuChannel) startWebhook(ctx context.Context) error {
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

// handleChallenge responds to Feisal's URL verification request.
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
		SenderName: msgEvent.Sender.SenderID.OpenID,
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

// dedup for webhook mode
type dedupEntry struct {
	ts time.Time
}

var (
	webhookDedupMu  sync.Mutex
	webhookDedupMap = make(map[string]dedupEntry)
)

// isDuplicate checks if an event has already been processed (dedup).
func (c *feishuChannel) isDuplicate(eventID string) bool {
	webhookDedupMu.Lock()
	defer webhookDedupMu.Unlock()

	if entry, exists := webhookDedupMap[eventID]; exists {
		// Check if entry is still valid (within 10 minutes)
		if time.Since(entry.ts) < 10*time.Minute {
			return true
		}
		// Entry expired, remove it
		delete(webhookDedupMap, eventID)
	}
	webhookDedupMap[eventID] = dedupEntry{ts: time.Now()}

	// Cleanup old entries periodically
	if len(webhookDedupMap) > 1000 {
		cutoff := time.Now().Add(-10 * time.Minute)
		for id, entry := range webhookDedupMap {
			if entry.ts.Before(cutoff) {
				delete(webhookDedupMap, id)
			}
		}
	}

	return false
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
	channel  *feishuChannel
	renderer *MarkdownRenderer
}

func newFeishuOutbound(ch *feishuChannel) *feishuOutbound {
	ob := &feishuOutbound{channel: ch}
	// Initialize the MarkdownRenderer with the channel's token provider and domain.
	ob.renderer = NewMarkdownRenderer(ch.getTenantToken, ch.cfg.Domain)
	return ob
}

func (o *feishuOutbound) SendText(ctx context.Context, chatID string, text string, opts *gateway.SendOptions) error {
	content := map[string]string{"text": text}
	contentJSON, _ := json.Marshal(content)

	_, err := o.sendMessage(ctx, chatID, "text", string(contentJSON), opts)
	return err
}

func (o *feishuOutbound) SendMarkdown(ctx context.Context, chatID string, markdown string, opts *gateway.SendOptions) error {
	// Smart message type selection:
	// - Code Blocks, tables, images -> interactive card (rich rendering)
	// - Plain text/lists -> post message (more natural for conversations)
	if needsCardRendering(markdown) {
		return o.sendAsCard(ctx, chatID, markdown, opts)
	}

	return o.sendAsPost(ctx, chatID, markdown, opts)
}

// needsCardRendering checks if the markdown content contains elements that require
// interactive card rendering (code blocks). Post message cannot render these properly.
func needsCardRendering(markdown string) bool {
	return strings.Contains(markdown, "```")
}

// sendAsCard sends markdown as an interactive card message.
// Used when content contains code blocks or other elements requiring rich rendering.
func (o *feishuOutbound) sendAsCard(ctx context.Context, chatID string, markdown string, opts *gateway.SendOptions) error {
	card := o.renderer.RenderToCard(markdown)
	cardJSON, _ := json.Marshal(card)

	logger.Debug("[Feishu] sending interactive card message, content: %s", string(cardJSON))

	_, err := o.sendMessage(ctx, chatID, "interactive", string(cardJSON), opts)
	return err
}

// sendAsPost sends Markdown as a post (rich text) message.
// Used the MarkdownRenderer pipeline for preprocessing:
// code block protection -> table-to-bullets conversion -> post structure build.
func (o *feishuOutbound) sendAsPost(ctx context.Context, chatID string, markdown string, opts *gateway.SendOptions) error {
	post := o.renderer.RenderToPost(markdown)
	contentJSON, _ := json.Marshal(post)

	logger.Debug("[Feishu] sending post message, content: %s", string(contentJSON))

	_, err := o.sendMessage(ctx, chatID, "post", string(contentJSON), opts)
	return err
}

// sendMessage sends a message via the Feishu/Lark IM API.
// Returns the message ID if successful.
func (o *feishuOutbound) sendMessage(ctx context.Context, chatID, msgType, content string, opts *gateway.SendOptions) (string, error) {
	token, err := o.channel.getTenantToken(ctx)
	if err != nil {
		return "", fmt.Errorf("get tenant token: %w", err)
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

	// Select URL based on domain
	sendURL := feishuSendMessageURL
	if o.channel.cfg.Domain == DomainLark {
		sendURL = larkSendMessageURL
	}

	url := sendURL + "?receive_id_type=chat_id"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("feishu API error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	// Parse response to get message ID
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Return empty ID if parsing fails, but don't fail to send
		return "", nil
	}

	if result.Code != 0 {
		logger.Warn("[Feishu] API returned error: code=%d, msg=%s", result.Code, result.Msg)
		return "", fmt.Errorf("feishu API error: code=%d, msg=%s", result.Code, result.Msg)
	}

	return result.Data.MessageID, nil
}

// AddReaction adds an emoji reaction to a message.
// The emojiType should be a valid Feishu emoji type like "OnIt", "OK", "THUMBSUP", etc.
// Returns the reaction_id for later removal.
// See: https://open.feishu.cn/document/server-docs/im-v1/message-reaction/create
func (o *feishuOutbound) AddReaction(ctx context.Context, messageID, emojiType string) (string, error) {
	token, err := o.channel.getTenantToken(ctx)
	if err != nil {
		return "", fmt.Errorf("get tenant token: %w", err)
	}

	// Feishu reaction API requires nested structure:
	// {"reaction_type": {"emoji_type": "THUMBSUP"}}
	payload := map[string]interface{}{
		"reaction_type": map[string]string{
			"emoji_type": emojiType,
		},
	}
	payloadJSON, _ := json.Marshal(payload)

	// Select URL based on domain
	reactionURL := feishuReactionURL
	if o.channel.cfg.Domain == DomainLark {
		reactionURL = larkReactionURL
	}

	url := reactionURL + "/" + messageID + "/reactions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("add reaction: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("feishu reaction API error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	// Parse response to get reaction_id for later removal.
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ReactionID string `json:"reaction_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		logger.Debug("[Feishu] failed to parse reaction response: %v", err)
		return "", nil
	}

	return result.Data.ReactionID, nil
}

// RemoveReaction removes an emoji reaction from a message.
// Note: Feishu DELETE reaction API requires the reaction_id, not emoji_type.
// Since we don't track reaction_id, we skip removal for now.
// The reaction will remain on the user's message as a record of processing.
func (o *feishuOutbound) RemoveReaction(ctx context.Context, messageID, reactionID string) error {
	if reactionID == "" {
		return nil
	}

	token, err := o.channel.getTenantToken(ctx)
	if err != nil {
		return fmt.Errorf("get tenant token: %w", err)
	}

	// Select URL based on domain
	reactionURL := feishuReactionURL
	if o.channel.cfg.Domain == DomainLark {
		reactionURL = larkReactionURL
	}

	// DELETE /open-apis/im/v1/messages/:message_id/reactions/:reaction_id
	url := reactionURL + "/" + messageID + "/reactions/" + reactionID
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("remove reaction: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("feishu reaction API error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

// getTenantToken obtains or refreshes the Feishu/Lark tenant access token.
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

	// Select URL based on domain
	tokenURL := feishuTenantTokenURL
	if c.cfg.Domain == DomainLark {
		tokenURL = larkTenantTokenURL
	}

	payload, _ := json.Marshal(map[string]string{
		"app_id":     c.cfg.AppID,
		"app_secret": c.cfg.AppSecret,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(payload))
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

// buildMarkdownCard builds a Feishu interactive card from parsed CardContent.
//
// Card structure:
//   - header: card title (extracted from first Markdown heading), with template color
//   - elements: Markdown content block(s)
//
// Feishu card Markdown element supports:
//   - Bold (**), Italic (*), Strikethrough (~~)
//   - Links [text](url)
//   - Ordered/unordered lists
//   - Code blocks (```) with language highlight
//   - Inline code (`code`)
//
// Does NOT support: # headings, > blockquotes, tables.
func buildMarkdownCard(content *CardContent) map[string]interface{} {
	card := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
	}

	// Set card header if title was extracted from markdown
	if content.Title != "" {
		card["header"] = map[string]interface{}{
			"title": map[string]interface{}{
				"content": content.Title,
				"tag":     "plain_text",
			},
			"template": "blue",
		}
	}

	// Build elements
	elements := []map[string]interface{}{}

	mdContent := strings.TrimSpace(content.Markdown)
	if mdContent != "" {
		elements = append(elements, map[string]interface{}{
			"tag":     "markdown",
			"content": mdContent,
		})
	}

	// Ensure at least one element
	if len(elements) == 0 {
		elements = append(elements, map[string]interface{}{
			"tag":     "markdown",
			"content": " ",
		})
	}

	card["elements"] = elements

	return card
}

var _ gateway.Channel = (*feishuChannel)(nil)
var _ gateway.OutboundAdapter = (*feishuOutbound)(nil)
