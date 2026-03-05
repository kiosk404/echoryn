package registry

import (
	"time"
)

// Config holds Registry configuration.
type Config struct {
	HeartbeatTimeout time.Duration
	CleanupInternal  time.Duration
	MaxNodes         int
}

// CompletedConfig is the validated configuration.
type CompletedConfig struct {
	*Config
}

// Complete fills in default values
func (c *Config) Complete() *CompletedConfig {
	if c.HeartbeatTimeout == 0 {
		c.HeartbeatTimeout = 45 * time.Second
	}
	if c.CleanupInternal == 0 {
		c.CleanupInternal = 60 * time.Second
	}
	if c.MaxNodes == 0 {
		c.MaxNodes = 100
	}
	return &CompletedConfig{}
}

// New creates a new Registry
func (c *CompletedConfig) New() (Registry, error) {
	return NewInMemoryRegistry(c.Config), nil
}
