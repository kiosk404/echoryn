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
//
// It buffers text deltas until the stream completes, then sends the
// full response as a single message (or multiple messages if chunking
// is needed for platform limits).
type Deliverer struct {
	channelManager *ChannelManager
}

// NewDeliverer creates a new Deliverer.
func NewDeliverer(manager *ChannelManager) *Deliverer {
	return &Deliverer{channelManager: manager}
}

// Deliver consumes the AgentEvent stream and sends the response to the IM platform.
// This method blocks until the stream is fully consumed.
func (d *Deliverer) Deliver(ctx context.Context, msg *InboundMessage, sr *schema.StreamReader[*entity.AgentEvent]) {
	if sr == nil {
		return
	}

	outbound, ok := d.channelManager.GetOutbound(msg.ChannelID)
	if !ok {
		logger.Warn("[Gateway] no outbound adapter for channel %s", msg.ChannelID)
		return
	}

	// Accumulate the full response text from text_delta events.
	var textBuf strings.Builder
	var hasError bool
	var errorMsg string

	for {
		event, err := sr.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Warn("[Gateway] stream read error: channel=%s, chat=%s, err=%v",
				msg.ChannelID, msg.ChatID, err)
			hasError = true
			errorMsg = err.Error()
			break
		}

		switch event.Type {
		case entity.EventTextDelta:
			textBuf.WriteString(event.Delta)

		case entity.EventError:
			hasError = true
			errorMsg = event.Error

		case entity.EventDone:
			// Stream completed normally.

		case entity.EventToolCallStart:
			// Log tool calls but don't send to IM (too noisy).
			if event.ToolCall != nil {
				logger.Debug("[Gateway] tool call: %s", event.ToolCall.Name)
			}

		case entity.EventToolCallEnd:
			// Tool completed — could optionally notify the user.
		}
	}

	// Send the accumulated response.
	responseText := strings.TrimSpace(textBuf.String())

	if hasError && responseText == "" {
		responseText = "⚠️ 处理请求时出错"
		if errorMsg != "" {
			responseText += ": " + errorMsg
		}
	}

	if responseText == "" {
		logger.Debug("[Gateway] empty response, skipping delivery: channel=%s, chat=%s",
			msg.ChannelID, msg.ChatID)
		return
	}

	opts := &SendOptions{
		ReplyTo: msg.Extra["message_id"],
	}

	// Try Markdown first, fall back to plain text.
	if err := outbound.SendMarkdown(ctx, msg.ChatID, responseText, opts); err != nil {
		logger.Warn("[Gateway] markdown delivery failed, trying plain text: %v", err)
		if err2 := outbound.SendText(ctx, msg.ChatID, responseText, opts); err2 != nil {
			logger.Error("[Gateway] delivery failed: channel=%s, chat=%s, err=%v",
				msg.ChannelID, msg.ChatID, err2)
		}
	}

	logger.Info("[Gateway] delivered response: channel=%s, chat=%s, len=%d",
		msg.ChannelID, msg.ChatID, len(responseText))
}
