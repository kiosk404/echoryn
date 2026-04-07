package diagnostics

import (
	diagentity "github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin/diagnostics/entity"
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
)

// ResolveDiagnosticsConfig extracts diagnostics config from PluginsOptions.entries["diagnostics"].config,
// falling back to defaults if not specified.
func ResolveDiagnosticsConfig(opts *genericoptions.PluginsOptions) *diagentity.DiagnosticsConfig {
	cfg := diagentity.DefaultDiagnosticsConfig()
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
	if v, ok := entry.Config["otel_endpoint"]; ok {
		if s, ok := v.(string); ok {
			cfg.OTEL.Endpoint = s
		}
	}
	if v, ok := entry.Config["otel_enabled"]; ok {
		if b, ok := v.(bool); ok {
			cfg.OTEL.Enabled = b
		}
	}
	if v, ok := entry.Config["enable_traces"]; ok {
		if b, ok := v.(bool); ok {
			cfg.OTEL.Traces = b
		}
	}
	if v, ok := entry.Config["enable_metrics"]; ok {
		if b, ok := v.(bool); ok {
			cfg.OTEL.Metrics = b
		}
	}
	if v, ok := entry.Config["enable_logs"]; ok {
		if b, ok := v.(bool); ok {
			cfg.OTEL.Logs = b
		}
	}
	if v, ok := entry.Config["service_name"]; ok {
		if s, ok := v.(string); ok {
			cfg.OTEL.ServiceName = s
		}
	}

	return cfg
}
