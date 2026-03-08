package tokenmanager

import (
	"time"
)

// Config holds TokenManager configuration
type Config struct {
	DefaultTTL    time.Duration
	MaxTTL        time.Duration
	CleanupPeriod time.Duration
	StorePath     string // BoltDB file path (empty = in-memory only)
}

// CompletedConfig is the validated configuration.
type CompletedConfig struct {
	*Config
}

// Complete fills in default values.
func (c *Config) Complete() *CompletedConfig {
	if c.DefaultTTL == 0 {
		c.DefaultTTL = 24 * time.Hour
	}
	if c.MaxTTL == 0 {
		c.MaxTTL = 168 * time.Hour // 7 days
	}
	if c.CleanupPeriod == 0 {
		c.CleanupPeriod = time.Hour
	}
	return &CompletedConfig{c}
}

// New creates a new TokenManager.
func (c *CompletedConfig) New() (TokenManager, error) {
	return newManager(c.Config)
}
