package gateway

import (
	"context"
	"io"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// Deliverer consumes an AgentEvent stream and delivers the accumulated
// assistant response back to the IM platform via the OutboundAdapter.
type Deliverer struct {
	channelManager *ChannelManager
}

// NewDeliverer creates a new Deliverer.
func NewDeliverer(manager *ChannelManager) *Deliverer {
	return &Deliverer{channelManager: manager}
}

// DeliverTrigger implements runtime.TriggerDeliverer interface.
// Called by AgentRunner.triggerAgentTurn when a sub-agent completes
// and triggers a new turn on the parent session.
func (d *Deliverer) DeliverTrigger(ctx context.Context, sessionID string, sr *schema.StreamReader[*entity.AgentEvent]) {
	channelID, chatID := parseSessionID(sessionID)
	if channelID == "" || chatID == "" {
		logger.Debug("[Gateway] DeliverTrigger: sessionID %q is not an IM session, skipping", sessionID)
		return
	}
	d.deliver(ctx, channelID, chatID, nil, sr)
}

// Deliver consumes the AgentEvent stream and sends the response to the IM platform.
func (d *Deliverer) Deliver(ctx context.Context, msg *InboundMessage, sr *schema.StreamReader[*entity.AgentEvent]) {
	if sr == nil || msg == nil {
		return
	}
	opts := &SendOptions{ReplyTo: msg.Extra["message_id"]}
	d.deliver(ctx, msg.ChannelID, msg.ChatID, opts, sr)
}

// deliver is the core implementation that consumes an AgentEvent stream
// and sends the accumulated response to the IM channel.
func (d *Deliverer) deliver(ctx context.Context, channelID, chatID string, opts *SendOptions, sr *schema.StreamReader[*entity.AgentEvent]) {
	if sr == nil {
		return
	}

	outbound, ok := d.channelManager.GetOutbound(channelID)
	if !ok {
		logger.Warn("[Gateway] no outbound adapter for channel %s", channelID)
		return
	}

	// Consume stream and accumulate response.
	responseText, hasError, errorMsg := consumeStream(sr)

	if hasError && responseText == "" {
		responseText = "⚠️ 处理请求时出错"
		if errorMsg != "" {
			responseText += ": " + errorMsg
		}
	}

	if responseText == "" {
		logger.Debug("[Gateway] empty response, skipping: channel=%s, chat=%s", channelID, chatID)
		return
	}

	// Try Markdown first, fall back to plain text.
	if err := outbound.SendMarkdown(ctx, chatID, responseText, opts); err != nil {
		logger.Warn("[Gateway] markdown failed, trying plain text: %v", err)
		if err2 := outbound.SendText(ctx, chatID, responseText, opts); err2 != nil {
			logger.Error("[Gateway] delivery failed: channel=%s, chat=%s, err=%v", channelID, chatID, err2)
		}
	}

	logger.Info("[Gateway] delivered: channel=%s, chat=%s, len=%d", channelID, chatID, len(responseText))
}

// consumeStream reads all events from the stream and returns the accumulated text.
func consumeStream(sr *schema.StreamReader[*entity.AgentEvent]) (text string, hasError bool, errorMsg string) {
	var buf strings.Builder
	for {
		event, err := sr.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return buf.String(), true, err.Error()
		}

		switch event.Type {
		case entity.EventTextDelta:
			buf.WriteString(event.Delta)
		case entity.EventError:
			return buf.String(), true, event.Error
		case entity.EventToolCallStart:
			if event.ToolCall != nil {
				logger.Debug("[Gateway] tool call: %s", event.ToolCall.Name)
			}
		}
	}
	return strings.TrimSpace(buf.String()), false, ""
}

// parseSessionID extracts channelID and chatID from a sessionID.
// IM sessions use format "{channel_id}:{chat_id}".
func parseSessionID(sessionID string) (channelID, chatID string) {
	parts := strings.SplitN(sessionID, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
