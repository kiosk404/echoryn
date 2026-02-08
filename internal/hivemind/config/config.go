package config

import (
	"github.com/kiosk404/eidolon/internal/hivemind/options"
)

// Config is the running configuration structure of the Ultronix service.
type Config struct {
	*options.Options
}

// CreateConfigFromOptions creates a running configuration instance based
func CreateConfigFromOptions(opts *options.Options) (*Config, error) {
	return &Config{opts}, nil
}
