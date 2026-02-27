package util

import (
	"context"
)

// Factory provides abstractions that allow the echoctl command to be extended across multiple types
// of resources and different API sets.
// The rings are here for a reason. In order for composers to be able to provide alternative factory implementations
// they need to provide low level pieces of *certain* functions so that when the factory calls back into itself
// it uses the custom version of the function. Rather than try to enumerate everything that someone would want to
// override
// we split the factory into rings, where each ring can depend on methods in an earlier ring, but cannot depend
// upon peer methods in its own ring.
// commands are decoupled from the factory).
type Factory interface {
	ClusterInitializer() ClusterInitializer
	ConfigManager() ConfigManager
	NodeDiagnostics() NodeDiagnostics
}

// ClusterInitializer provides cluster node initialization capabilities.
type ClusterInitializer interface {
	InitHivemind(ctx context.Context, opts HivemindInitOpts) error
	InitGolem(ctx context.Context, opts GolemInitOpts) error
}

// HivemindInitOpts holds options for initializing a Hivemind control plane node.
type HivemindInitOpts struct {
	BindAddress string
	BindPort    int
	ConfigDir   string
}

// GolemInitOpts holds options for initializing a Golem worker node.
type GolemInitOpts struct {
	Workspace  string
	PluginsDir string
}

// ConfigManager provides configuration management capabilities.
type ConfigManager interface {
	Load(ctx context.Context, path string) (any, error)
	Save(ctx context.Context, path string, cfg any) error
	Validate(ctx context.Context, path string) ([]string, error)
}

// NodeDiagnostics provides node diagnostic information collection.
type NodeDiagnostics interface {
	CollectInfo(ctx context.Context) (*NodeInfo, error)
}

// NodeInfo holds collected node diagnostic information.
type NodeInfo struct {
	NodeRole  string
	NodeName  string
	NodeID    string
	HostName  string
	IPAddress string
	OSRelease string
	CPUCore   uint64
	MemTotal  string
	MemFree   string
	Version   string
}

type defaultFactory struct{}

// NewDefaultFactory creates a new default factory instance.
func NewDefaultFactory() Factory {
	return &defaultFactory{}
}

func (d *defaultFactory) ClusterInitializer() ClusterInitializer {
	return &defaultClusterInitializer{}
}

func (d *defaultFactory) ConfigManager() ConfigManager {
	return &defaultConfigManager{}
}

func (d *defaultFactory) NodeDiagnostics() NodeDiagnostics {
	return &defaultNodeDiagnostics{}
}

type defaultClusterInitializer struct{}

func (d *defaultClusterInitializer) InitHivemind(ctx context.Context, opts HivemindInitOpts) error {
	//TODO implement me
	panic("implement me")
}

func (d *defaultClusterInitializer) InitGolem(ctx context.Context, opts GolemInitOpts) error {
	//TODO implement me
	panic("implement me")
}

type defaultConfigManager struct{}

func (d *defaultConfigManager) Load(ctx context.Context, path string) (any, error) {
	//TODO implement me
	panic("implement me")
}

func (d *defaultConfigManager) Save(ctx context.Context, path string, cfg any) error {
	//TODO implement me
	panic("implement me")
}

func (d *defaultConfigManager) Validate(ctx context.Context, path string) ([]string, error) {
	//TODO implement me
	panic("implement me")
}

type defaultNodeDiagnostics struct{}

func (d *defaultNodeDiagnostics) CollectInfo(ctx context.Context) (*NodeInfo, error) {
	//TODO implement me
	panic("implement me")
}
