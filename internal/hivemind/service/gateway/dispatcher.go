package gateway

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// Dispatcher handles inbound messages from IM channels and routes them
// to the appropriate Agent session via AgentService.Run().
//
// It implements InboundHandler and is the bridge between the IM world
// and the Agent runtime world.
//
// Message flow:
//
//	Channel → Dispatcher.HandleMessage() → AgentService.Run()
//	                                        → Deliverer.Deliver()
//	                                          → OutboundAdapter.Send*()
type Dispatcher struct {
	agentSvc       service.AgentService
	channelManager *ChannelManager
	defaultAgentID string
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(agentSvc service.AgentService, manager *ChannelManager, defaultAgentID string) *Dispatcher {
	return &Dispatcher{
		agentSvc:       agentSvc,
		channelManager: manager,
		defaultAgentID: defaultAgentID,
	}
}

// SetChannelManager sets or updates the channel manager reference.
// This allows deferred wiring when the manager is created after the dispatcher.
func (d *Dispatcher) SetChannelManager(manager *ChannelManager) {
	d.channelManager = manager
}

// HandleMessage implements InboundHandler. It maps the inbound message to
// an Agent session and invokes AgentService.Run(), then delivers the response
// back to the IM platform.
func (d *Dispatcher) HandleMessage(ctx context.Context, msg *InboundMessage) error {
	if msg == nil || msg.Text == "" {
		return nil
	}

	// 1. Map channel + chatID → session ID.
	sessionID := d.resolveSessionID(msg)

	// 2. Resolve which agent to route to.
	agentID := d.resolveAgentID(msg.ChannelID)

	logger.Info("[Gateway] dispatching message: channel=%s, chat=%s, sender=%s, agent=%s, session=%s",
		msg.ChannelID, msg.ChatID, msg.SenderName, agentID, sessionID)

	// 2.5. Add a "working" emoji reaction to the user's message to indicate processing.
	var reactionID string
	var reactionMsgID string
	if outbound, ok := d.channelManager.GetOutbound(msg.ChannelID); ok {
		if userMsgID := msg.Extra["message_id"]; userMsgID != "" {
			if rid, err := outbound.AddReaction(ctx, userMsgID, "OnIt"); err != nil {
				logger.Debug("[Gateway] failed to add working reaction: %v", err)
			} else {
				reactionID = rid
				reactionMsgID = userMsgID
			}
		}
	}

	// 3. Invoke AgentService.Run().
	streamReader, err := d.agentSvc.Run(ctx, &runtime.RunRequest{
		AgentID:   agentID,
		SessionID: sessionID,
		Input:     msg.Text,
	})
	if err != nil {
		logger.Error("[Gateway] agent run failed: channel=%s, chat=%s, err=%v", msg.ChannelID, msg.ChatID, err)
		d.sendErrorReply(ctx, msg, err)
		// Remove working reaction on error
		d.removeWorkingReaction(ctx, msg.ChannelID, reactionMsgID, reactionID)
		return fmt.Errorf("agent run failed: %w", err)
	}

	// 4. Consume the event stream and deliver response.
	go d.consumeAndDeliver(ctx, msg, streamReader, reactionMsgID, reactionID)

	return nil
}

// resolveSessionID maps channel + chatID to a deterministic session ID.
// Format: "{channel_id}:{chat_id}" — ensures each IM conversation maps to
// exactly one Agent session.
func (d *Dispatcher) resolveSessionID(msg *InboundMessage) string {
	return fmt.Sprintf("%s:%s", msg.ChannelID, msg.ChatID)
}

// resolveAgentID determines which agent should handle messages from this channel.
func (d *Dispatcher) resolveAgentID(channelID string) string {
	if agentID := d.channelManager.GetAgentID(channelID); agentID != "" {
		return agentID
	}
	return d.defaultAgentID
}

// consumeAndDeliver reads the AgentEvent stream and sends the accumulated
// response back to the IM platform. After delivery, removes the working reaction.
func (d *Dispatcher) consumeAndDeliver(ctx context.Context, msg *InboundMessage, sr *schema.StreamReader[*entity.AgentEvent], reactionMsgID, reactionID string) {
	deliverer := NewDeliverer(d.channelManager)
	deliverer.Deliver(ctx, msg, sr)

	// Remove the "working" reaction after delivery is complete.
	d.removeWorkingReaction(ctx, msg.ChannelID, reactionMsgID, reactionID)
}

// removeWorkingReaction removes the working indicator reaction from a message.
func (d *Dispatcher) removeWorkingReaction(ctx context.Context, channelID, messageID, reactionID string) {
	if reactionID == "" || messageID == "" {
		return
	}
	outbound, ok := d.channelManager.GetOutbound(channelID)
	if !ok {
		return
	}
	if err := outbound.RemoveReaction(ctx, messageID, reactionID); err != nil {
		logger.Debug("[Gateway] failed to remove working reaction: %v", err)
	}
}

// sendErrorReply sends an error message back to the IM platform.
func (d *Dispatcher) sendErrorReply(ctx context.Context, msg *InboundMessage, err error) {
	outbound, ok := d.channelManager.GetOutbound(msg.ChannelID)
	if !ok {
		return
	}

	errText := fmt.Sprintf("⚠️ 处理消息时出错: %v", err)
	if sendErr := outbound.SendText(ctx, msg.ChatID, errText, nil); sendErr != nil {
		logger.Warn("[Gateway] failed to send error reply: %v", sendErr)
	}
}

// Ensure Dispatcher implements InboundHandler.
var _ InboundHandler = (*Dispatcher)(nil)
