package node

import (
	"time"
)

// Config holds configuration for the Golem node service
type Config struct {
	NodeName           string
	NodeLabels         map[string]string
	HivemindAddress    string // Hivemind gRPC address to connect to
	HeartbeatInterval  time.Duration
	ConnectTimeout     time.Duration
	ReconnectInterval  time.Duration
	MaxConcurrentTasks int32
	WorkspaceDir       string
	SkillsDir          string
	JoinToken          string // Bootstrap Token for registration; empty = no auth (local dev)
	FileOpsEnabled     bool   // Advertise file operations capability file_read, file_write, file_patch
}

// CompletedConfig is the result of calling Complete on Config.
type CompletedConfig struct {
	*Config
}

// Complete fills in default values for any unset fields.
func (c *Config) Complete() *CompletedConfig {
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = 15 * time.Second
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	if c.ReconnectInterval == 0 {
		c.ReconnectInterval = 5 * time.Second
	}
	if c.MaxConcurrentTasks == 0 {
		c.MaxConcurrentTasks = 5
	}
	return &CompletedConfig{c}
}

// New creates a new node Service instance.
func (c *CompletedConfig) New() (*Service, error) {
	return NewService(c.Config)
}
