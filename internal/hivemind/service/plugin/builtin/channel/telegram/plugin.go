package telegram

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/gateway"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "channel-telegram"

	// Kind groups this plugin under the "channel" slot.
	Kind = "channel"
)

// PluginDefinition returns the static metadata for the Telegram channel plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Telegram Channel",
		Kind:        Kind,
		Description: "Telegram Bot API channel integration via long polling",
	}
}

// telegramPlugin is the runtime instance of the Telegram channel plugin.
type telegramPlugin struct {
	cfg      *TelegramConfig
	channel  *telegramChannel
	outbound *telegramOutbound
	manager  *gateway.ChannelManager
}

// ChannelManagerSetter is a K8s-style interface probe for injecting
// the ChannelManager into the plugin after module assembly.
type ChannelManagerSetter interface {
	SetChannelManager(m *gateway.ChannelManager)
}

// Factory is the PluginFactory for channel-telegram.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfgRaw, ok := args["config"]
	if !ok {
		return nil, fmt.Errorf("channel-telegram: missing 'config' in plugin args")
	}
	cfg, ok := cfgRaw.(*TelegramConfig)
	if !ok {
		return nil, fmt.Errorf("channel-telegram: 'config' must be *TelegramConfig, got %T", cfgRaw)
	}

	ch := newTelegramChannel(cfg)
	ob := newTelegramOutbound(cfg)

	return &telegramPlugin{
		cfg:      cfg,
		channel:  ch,
		outbound: ob,
	}, nil
}

// Name implements plugin.Plugin.
func (p *telegramPlugin) Name() string { return PluginName }

// SetChannelManager implements ChannelManagerSetter.
// Called by the server layer to inject the shared ChannelManager.
func (p *telegramPlugin) SetChannelManager(m *gateway.ChannelManager) {
	p.manager = m
}

// Init implements plugin.InitPlugin.
// Registers lifecycle hooks for starting/stopping the channel.
func (p *telegramPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled {
		logger.Info("[ChannelTelegram] plugin disabled, skipping hook registration")
		return nil
	}

	api.RegisterHook(plugin.HookServerStart, p.onServerStart)
	api.RegisterHook(plugin.HookServerStop, p.onServerStop)

	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *telegramPlugin) Start(ctx context.Context) error {
	if !p.cfg.Enabled {
		logger.Info("[ChannelTelegram] plugin disabled")
		return nil
	}

	if p.cfg.BotToken == "" {
		return fmt.Errorf("channel-telegram: bot_token is required")
	}

	logger.Info("[ChannelTelegram] plugin started (polling_timeout=%ds)", p.cfg.PollingTimeout)
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *telegramPlugin) Stop(ctx context.Context) error {
	logger.Info("[ChannelTelegram] plugin stopped")
	return nil
}

// onServerStart is the HookServerStart handler.
func (p *telegramPlugin) onServerStart(ctx context.Context, data interface{}) error {
	if p.manager == nil {
		logger.Warn("[ChannelTelegram] no ChannelManager available, skipping channel start")
		return nil
	}

	p.manager.Register(p.channel, p.outbound, p.cfg.AgentID)
	logger.Info("[ChannelTelegram] registered channel with gateway manager")
	return nil
}

// onServerStop is the HookServerStop handler.
func (p *telegramPlugin) onServerStop(ctx context.Context, data interface{}) error {
	return nil
}

// Channel returns the underlying gateway.Channel for direct access.
func (p *telegramPlugin) Channel() gateway.Channel { return p.channel }

// Outbound returns the underlying gateway.OutboundAdapter.
func (p *telegramPlugin) Outbound() gateway.OutboundAdapter { return p.outbound }

// AgentID returns the configured agent ID for this channel.
func (p *telegramPlugin) AgentID() string { return p.cfg.AgentID }

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*telegramPlugin)(nil)
	_ plugin.InitPlugin      = (*telegramPlugin)(nil)
	_ plugin.LifecyclePlugin = (*telegramPlugin)(nil)
	_ ChannelManagerSetter   = (*telegramPlugin)(nil)
)
