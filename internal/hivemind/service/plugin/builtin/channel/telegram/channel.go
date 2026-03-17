package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kiosk404/echoryn/internal/hivemind/service/gateway"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// ChannelID is the unique identifier for the Telegram channel.
	ChannelID = "telegram"

	// telegramAPIBase is the Telegram Bot API base URL.
	telegramAPIBase = "https://api.telegram.org/bot"
)

// telegramChannel implements gateway.Channel for the Telegram platform.
//
// It uses the Telegram Bot API long polling (getUpdates) to receive messages,
// normalizes them into InboundMessage, and forwards them to the dispatcher.
//
// Outbound messages are sent via the Telegram sendMessage API.
type telegramChannel struct {
	cfg     *TelegramConfig
	handler gateway.InboundHandler
	cancel  context.CancelFunc

	// lastUpdateID tracks the last processed update to avoid duplicates.
	lastUpdateID int64

	// allowedChats is a set for fast lookup when chat restriction is enabled.
	allowedChats map[int64]struct{}
}

// newTelegramChannel creates a new Telegram channel.
func newTelegramChannel(cfg *TelegramConfig) *telegramChannel {
	ch := &telegramChannel{
		cfg:          cfg,
		allowedChats: make(map[int64]struct{}),
	}
	for _, id := range cfg.AllowedChatIDs {
		ch.allowedChats[id] = struct{}{}
	}
	return ch
}

func (c *telegramChannel) ID() string { return ChannelID }

func (c *telegramChannel) Start(ctx context.Context, handler gateway.InboundHandler) error {
	c.handler = handler

	pollCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	go c.pollLoop(pollCtx)

	logger.Info("[Telegram] bot started with long polling (timeout=%ds)", c.cfg.PollingTimeout)
	return nil
}

func (c *telegramChannel) Stop(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	logger.Info("[Telegram] bot stopped")
	return nil
}

// pollLoop runs the Telegram long polling loop.
func (c *telegramChannel) pollLoop(ctx context.Context) {
	timeout := c.cfg.PollingTimeout
	if timeout <= 0 {
		timeout = 30
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := c.getUpdates(ctx, c.lastUpdateID+1, timeout)
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled, normal shutdown.
			}
			logger.Warn("[Telegram] getUpdates error: %v, retrying in 5s", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		for _, update := range updates {
			if update.UpdateID >= c.lastUpdateID {
				c.lastUpdateID = update.UpdateID
			}

			c.processUpdate(ctx, &update)
		}
	}
}

// processUpdate handles a single Telegram Update.
func (c *telegramChannel) processUpdate(ctx context.Context, update *telegramUpdate) {
	msg := update.Message
	if msg == nil {
		return
	}

	// Check chat restriction.
	if len(c.allowedChats) > 0 {
		if _, ok := c.allowedChats[msg.Chat.ID]; !ok {
			logger.Debug("[Telegram] ignoring message from restricted chat: %d", msg.Chat.ID)
			return
		}
	}

	// Only handle text messages.
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	// Remove /command prefix if it's a bot command.
	text = cleanBotCommand(text)
	if text == "" {
		return
	}

	// Build InboundMessage.
	chatIDStr := strconv.FormatInt(msg.Chat.ID, 10)
	senderID := strconv.FormatInt(msg.From.ID, 10)
	senderName := msg.From.FirstName
	if msg.From.LastName != "" {
		senderName += " " + msg.From.LastName
	}
	if msg.From.Username != "" {
		senderName = "@" + msg.From.Username
	}

	inMsg := &gateway.InboundMessage{
		ChannelID:  ChannelID,
		ChatID:     chatIDStr,
		SenderID:   senderID,
		SenderName: senderName,
		Text:       text,
		Timestamp:  time.Unix(int64(msg.Date), 0),
		Extra: map[string]string{
			"message_id": strconv.Itoa(msg.MessageID),
			"chat_type":  msg.Chat.Type,
		},
	}

	if err := c.handler.HandleMessage(ctx, inMsg); err != nil {
		logger.Error("[Telegram] dispatch failed: chat=%s, err=%v", chatIDStr, err)
	}
}

// getUpdates calls the Telegram Bot API getUpdates endpoint.
func (c *telegramChannel) getUpdates(ctx context.Context, offset int64, timeout int) ([]telegramUpdate, error) {
	apiURL := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=%d&allowed_updates=[\"message\"]",
		telegramAPIBase, c.cfg.BotToken, offset, timeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	// Use a client with a timeout slightly longer than the long polling timeout.
	client := &http.Client{
		Timeout: time.Duration(timeout+10) * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getUpdates request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getUpdates HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result telegramAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode getUpdates response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("getUpdates API error: %d %s", result.ErrorCode, result.Description)
	}

	return result.Result, nil
}

// cleanBotCommand strips the /start or other command prefixes,
// returning the remaining text (if any).
func cleanBotCommand(text string) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}
	// Common bot commands to ignore completely.
	cmd := strings.Fields(text)[0]
	switch cmd {
	case "/start", "/help":
		// These are informational commands, skip.
		return ""
	}
	// For other commands, strip the command and return the rest.
	parts := strings.SplitN(text, " ", 2)
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// --- Telegram API Types ---

type telegramAPIResponse struct {
	OK          bool             `json:"ok"`
	ErrorCode   int              `json:"error_code,omitempty"`
	Description string           `json:"description,omitempty"`
	Result      []telegramUpdate `json:"result,omitempty"`
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message,omitempty"`
}

type telegramMessage struct {
	MessageID int          `json:"message_id"`
	From      telegramUser `json:"from"`
	Chat      telegramChat `json:"chat"`
	Date      int          `json:"date"`
	Text      string       `json:"text"`
}

type telegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type telegramChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

// --- Outbound: Telegram sendMessage API ---

// telegramOutbound implements gateway.OutboundAdapter for sending messages via Telegram Bot API.
type telegramOutbound struct {
	cfg *TelegramConfig
}

func newTelegramOutbound(cfg *TelegramConfig) *telegramOutbound {
	return &telegramOutbound{cfg: cfg}
}

func (o *telegramOutbound) SendText(ctx context.Context, chatID string, text string, opts *gateway.SendOptions) error {
	return o.sendMessage(ctx, chatID, text, "")
}

func (o *telegramOutbound) SendMarkdown(ctx context.Context, chatID string, markdown string, opts *gateway.SendOptions) error {
	return o.sendMessage(ctx, chatID, markdown, "MarkdownV2")
}

// sendMessage sends a message via the Telegram Bot API.
func (o *telegramOutbound) sendMessage(ctx context.Context, chatID, text, parseMode string) error {
	apiURL := fmt.Sprintf("%s%s/sendMessage", telegramAPIBase, o.cfg.BotToken)

	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	if parseMode != "" {
		form.Set("parse_mode", parseMode)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sendMessage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// If MarkdownV2 parse failed, retry without parse mode.
		if parseMode == "MarkdownV2" && resp.StatusCode == http.StatusBadRequest {
			logger.Warn("[Telegram] MarkdownV2 send failed, retrying as plain text: %s", string(body))
			return o.sendMessage(ctx, chatID, text, "")
		}
		return fmt.Errorf("telegram API error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// AddReaction is not supported by Telegram Bot API in this implementation.
// Returns empty reactionID and nil error.
func (o *telegramOutbound) AddReaction(ctx context.Context, messageID string, emojiType string) (string, error) {
	// Telegram Bot API does support reactions (setMessageReaction) but
	// the implementation is deferred for now.
	return "", nil
}

// RemoveReaction is not supported by Telegram Bot API in this implementation.
func (o *telegramOutbound) RemoveReaction(ctx context.Context, messageID string, reactionID string) error {
	return nil
}

var _ gateway.Channel = (*telegramChannel)(nil)
var _ gateway.OutboundAdapter = (*telegramOutbound)(nil)
