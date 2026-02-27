package config

import (
	"fmt"
	"strings"

	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// GetByDotPath retrieves a value from the config using dot-notation path.
// Example: "models.default_provider" returns the default provider string.
func GetByDotPath(cfg *EchorynConfig, path string) (interface{}, error) {
	// Marshal config to generic map for dot-path traversal.
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return getFromMap(m, strings.Split(path, "."))
}

// SetByDotPath sets a value in the config using dot-notation path.
// The value is parsed as JSON first; if that fails, it's treated as a string.
func SetByDotPath(cfg *EchorynConfig, path string, value string) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	// Try to parse value as JSON (for booleans, numbers, objects).
	var parsedValue interface{}
	if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
		// Not valid JSON, treat as string.
		parsedValue = value
	}

	if err := setInMap(m, strings.Split(path, "."), parsedValue); err != nil {
		return err
	}

	// Marshal back and unmarshal into the config struct.
	data, err = json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal updated config: %w", err)
	}

	// Reset config to defaults then overlay.
	*cfg = *DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("unmarshal updated config: %w", err)
	}

	return nil
}

// UnsetByDotPath removes a key from the config using dot-notation path.
func UnsetByDotPath(cfg *EchorynConfig, path string) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	if err := unsetInMap(m, strings.Split(path, ".")); err != nil {
		return err
	}

	data, err = json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal updated config: %w", err)
	}

	*cfg = *DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("unmarshal updated config: %w", err)
	}

	return nil
}

// SanitizedView returns a copy of the config with sensitive fields masked.
func SanitizedView(cfg *EchorynConfig) *EchorynConfig {
	// Deep copy via JSON round-trip.
	data, _ := json.Marshal(cfg)
	var copy EchorynConfig
	_ = json.Unmarshal(data, &copy)

	// Mask auth tokens.
	if copy.Hivemind.AuthToken != "" {
		copy.Hivemind.AuthToken = maskString(copy.Hivemind.AuthToken)
	}
	if copy.Golem.JoinToken != "" {
		copy.Golem.JoinToken = maskString(copy.Golem.JoinToken)
	}
	for name, p := range copy.Models.Providers {
		if p.APIKey != "" {
			p.APIKey = maskString(p.APIKey)
			copy.Models.Providers[name] = p
		}
	}

	return &copy
}

func maskString(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func getFromMap(m map[string]interface{}, keys []string) (interface{}, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	val, ok := m[keys[0]]
	if !ok {
		return nil, fmt.Errorf("key %q not found", keys[0])
	}

	if len(keys) == 1 {
		return val, nil
	}

	sub, ok := val.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("key %q is not an object", keys[0])
	}

	return getFromMap(sub, keys[1:])
}

func setInMap(m map[string]interface{}, keys []string, value interface{}) error {
	if len(keys) == 0 {
		return fmt.Errorf("empty path")
	}

	if len(keys) == 1 {
		m[keys[0]] = value
		return nil
	}

	sub, ok := m[keys[0]]
	if !ok {
		// Create intermediate map.
		sub = make(map[string]interface{})
		m[keys[0]] = sub
	}

	subMap, ok := sub.(map[string]interface{})
	if !ok {
		// Overwrite non-map value with a new map.
		subMap = make(map[string]interface{})
		m[keys[0]] = subMap
	}

	return setInMap(subMap, keys[1:], value)
}

func unsetInMap(m map[string]interface{}, keys []string) error {
	if len(keys) == 0 {
		return fmt.Errorf("empty path")
	}

	if len(keys) == 1 {
		delete(m, keys[0])
		return nil
	}

	sub, ok := m[keys[0]]
	if !ok {
		return nil // Already doesn't exist.
	}

	subMap, ok := sub.(map[string]interface{})
	if !ok {
		return fmt.Errorf("key %q is not an object", keys[0])
	}

	return unsetInMap(subMap, keys[1:])
}
