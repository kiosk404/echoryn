package skills

import genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"

// ResolveSkillsConfig extracts skills config from PluginsOptions.entries["skills"].config,
// falling back to defaults if not specified.
func ResolveSkillsConfig(opts *genericoptions.PluginsOptions) *Config {
	cfg := DefaultConfig()
	if opts == nil {
		return cfg
	}

	entry, ok := opts.Entries[PluginName]
	if !ok || entry.Config == nil {
		return cfg
	}

	if v, ok := entry.Config["enabled"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Enabled = b
		}
	}
	if v, ok := entry.Config[""]; ok {
		if s, ok := v.(string); ok {
			cfg.HivemindSkillsDir = s
		}
	}
	if v, ok := entry.Config["global_skills_dir"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.GlobalSkillsDir = s
		}
	}
	if v, ok := entry.Config["project_skills_dir"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.ProjectSkillsDir = s
		}
	}
	if v, ok := entry.Config["hot_reload"]; ok {
		if b, ok := v.(bool); ok {
			cfg.HotReload = b
		}
	}

	return cfg
}
