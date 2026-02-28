package config

import (
	"fmt"
	"strings"

	"github.com/kiosk404/echoryn/pkg/paths"
)

// Validate checks the config for common errors and returns a list of issues.
func Validate(cfg *EchorynConfig) []string {
	var issues []string

	if cfg.Version == "" {
		issues = append(issues, "missing version field")
	}

	role := paths.NodeRole(cfg.Node.Role)
	if cfg.Node.Role != "" && role != paths.RoleHivemind && role != paths.RoleGolem {
		issues = append(issues, fmt.Sprintf("invalid node.role %q, must be 'hivemind' or 'golem'", cfg.Node.Role))
	}

	// Validate hivemind config if role is hivemind.
	if role == paths.RoleHivemind {
		if cfg.Hivemind.Address == "" {
			issues = append(issues, "hivemind.address is required when node.role is 'hivemind'")
		}
		if cfg.Hivemind.DataDir == "" {
			issues = append(issues, "hivemind.data_dir is required when node.role is 'hivemind'")
		}
	}

	// Validate golem config if role is golem.
	if role == paths.RoleGolem {
		if cfg.Golem.Workspace == "" {
			issues = append(issues, "golem.workspace is required when node.role is 'golem'")
		}
	}

	// Validate model provider keys (warn if env var placeholders are used).
	for name, p := range cfg.Models.Providers {
		if p.APIKey != "" && strings.HasPrefix(p.APIKey, "${") && strings.HasSuffix(p.APIKey, "}") {
			// Env var reference — acceptable but warn.
			issues = append(issues, fmt.Sprintf("models.providers.%s.api_key uses environment variable reference: %s", name, p.APIKey))
		}
	}

	return issues
}
