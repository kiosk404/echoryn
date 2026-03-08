package feishu

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/gateway"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/logger"
)

const (
	// PluginName is the unique identifier for this plugin.
	PluginName = "channel-feishu"

	// Kind groups this plugin under the "channel" slot.
	Kind = "channel"
)

// PluginDefinition returns the static metadata for the Feishu channel plugin.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Feishu Channel",
		Kind:        Kind,
		Description: "Feishu (Lark) IM channel integration via HTTP webhook or websocket + Bot API",
	}
}

// feishuPlugin is the runtime instance of the Feishu channel plugin.
type feishuPlugin struct {
	cfg      *FeishuConfig
	channel  *feishuChannel
	outbound *feishuOutbound
	manager  *gateway.ChannelManager
}

// ChannelManagerSetter is a K8s-style interface probe for injecting
// the ChannelManager into the plugin after module assembly.
type ChannelManagerSetter interface {
	SetChannelManager(m *gateway.ChannelManager)
}

// Factory is the PluginFactory for channel-feishu.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfgRaw, ok := args["config"]
	if !ok {
		return nil, fmt.Errorf("channel-feishu: missing 'config' in plugin args")
	}
	cfg, ok := cfgRaw.(*FeishuConfig)
	if !ok {
		return nil, fmt.Errorf("channel-feishu: 'config' must be *FeishuConfig, got %T", cfgRaw)
	}

	ch := newFeishuChannel(cfg)
	ob := newFeishuOutbound(ch)

	return &feishuPlugin{
		cfg:      cfg,
		channel:  ch,
		outbound: ob,
	}, nil
}

// Name implements plugin.Plugin.
func (p *feishuPlugin) Name() string { return PluginName }

// SetChannelManager implements ChannelManagerSetter.
// Called by the server layer to inject the shared ChannelManager.
func (p *feishuPlugin) SetChannelManager(m *gateway.ChannelManager) {
	p.manager = m
}

// Init implements plugin.InitPlugin.
// Registers lifecycle hooks for starting/stopping the channel.
func (p *feishuPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled {
		logger.Info("[ChannelFeishu] plugin disabled, skipping hook registration")
		return nil
	}

	// Register server_start hook to start the Feishu channel.
	api.RegisterHook(plugin.HookServerStart, p.onServerStart)

	// Register server_stop hook to stop the Feishu channel.
	api.RegisterHook(plugin.HookServerStop, p.onServerStop)

	return nil
}

// Start implements plugin.LifecyclePlugin.
func (p *feishuPlugin) Start(ctx context.Context) error {
	if !p.cfg.Enabled {
		logger.Info("[ChannelFeishu] plugin disabled")
		return nil
	}

	// Validate required configuration.
	if p.cfg.AppID == "" || p.cfg.AppSecret == "" {
		return fmt.Errorf("channel-feishu: app_id and app_secret are required")
	}

	mode := p.cfg.ConnectionMode
	if mode == "" {
		mode = ConnectionModeWebsocket
	}
	logger.Info("[ChannelFeishu] plugin started (mode=%s, domain=%s)", mode, p.cfg.Domain)
	return nil
}

// Stop implements plugin.LifecyclePlugin.
func (p *feishuPlugin) Stop(ctx context.Context) error {
	logger.Info("[ChannelFeishu] plugin stopped")
	return nil
}

// onServerStart is the HookServerStart handler.
// It registers the channel with the ChannelManager and starts it.
func (p *feishuPlugin) onServerStart(ctx context.Context, data interface{}) error {
	if p.manager == nil {
		logger.Warn("[ChannelFeishu] no ChannelManager available, skipping channel start")
		return nil
	}

	p.manager.Register(p.channel, p.outbound, p.cfg.AgentID)
	logger.Info("[ChannelFeishu] registered channel with gateway manager")
	return nil
}

// onServerStop is the HookServerStop handler.
// Channel shutdown is handled by ChannelManager.StopAll(), so this is a no-op.
func (p *feishuPlugin) onServerStop(ctx context.Context, data interface{}) error {
	return nil
}

// Channel returns the underlying gateway.Channel for direct access.
// This is exposed for server-layer integration (e.g., ChannelManager registration).
func (p *feishuPlugin) Channel() gateway.Channel { return p.channel }

// Outbound returns the underlying gateway.OutboundAdapter.
func (p *feishuPlugin) Outbound() gateway.OutboundAdapter { return p.outbound }

// AgentID returns the configured agent ID for this channel.
func (p *feishuPlugin) AgentID() string { return p.cfg.AgentID }

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*feishuPlugin)(nil)
	_ plugin.InitPlugin      = (*feishuPlugin)(nil)
	_ plugin.LifecyclePlugin = (*feishuPlugin)(nil)
	_ ChannelManagerSetter   = (*feishuPlugin)(nil)
)
