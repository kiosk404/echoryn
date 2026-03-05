package config

import (
	"github.com/kiosk404/echoryn/internal/golem/options"
)

// Config is the running configuration for a Golem worker node.
type Config struct {
	*options.Options
}

// CreateConfigFromOptions wraps validated Options into a Config
func CreateConfigFromOptions(opts *options.Options) (*Config, error) {
	return &Config{opts}, nil
}
