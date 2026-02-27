package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kiosk404/echoryn/pkg/utils/homedir"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

const (
	// DefaultConfigDir is the default echoryn configuration directory.
	DefaultConfigDir = ".echoryn"
	// ConfigFileName is the global config file name.
	ConfigFileName = "config.json"
)

// DefaultConfigPath returns the default path to config.json (~/.echoryn/config.json).
func DefaultConfigPath() string {
	return filepath.Join(homedir.HomeDir(), DefaultConfigDir, ConfigFileName)
}

// DefaultBaseDir returns the default echoryn base directory (~/.echoryn).
func DefaultBaseDir() string {
	return filepath.Join(homedir.HomeDir(), DefaultConfigDir)
}

// Load reads and parses the config file at the given path.
// If the file does not exist, it returns a default config.
func Load(path string) (*EchorynConfig, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}

// Save writes the config to the given path as formatted JSON.
// Parent directories are created if they don't exist.
func Save(path string, cfg *EchorynConfig) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// Exists returns true if the config file exists at the given path.
func Exists(path string) bool {
	if path == "" {
		path = DefaultConfigPath()
	}
	_, err := os.Stat(path)
	return err == nil
}
