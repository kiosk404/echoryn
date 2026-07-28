package localfileops

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/pkg/fileops"
	"github.com/kiosk404/echoryn/pkg/logger"
)

// PluginDefinition returns the static metadata describing this plugin to
// the framework.
func PluginDefinition() plugin.Definition {
	return plugin.Definition{
		ID:          PluginName,
		Name:        "Local File Operations",
		Kind:        "general",
		Description: "Read/write/patch/search files on the Hivemind local filesystem with sandbox enforcement.",
	}
}

// localFileOpsPlugin is the runtime instance of the plugin. It holds two
// distinct *fileops.Sandbox values: readSandbox (used by read_file and
// search_files) and writeSandbox (used by write_file and patch). This split
// lets the plugin register only the read tools when writes are disabled --
// the LLM never sees tools it cannot invoke.
type localFileOpsPlugin struct {
	cfg          *Config
	handle       plugin.Handle
	readSandbox  *fileops.Sandbox
	writeSandbox *fileops.Sandbox
}

// Factory constructs a plugin instance from the resolved config.
func Factory(args plugin.PluginArgs, handle plugin.Handle) (plugin.Plugin, error) {
	cfg := DefaultConfig()
	if c, ok := args["config"].(*Config); ok && c != nil {
		cfg = c
	}
	return &localFileOpsPlugin{cfg: cfg, handle: handle}, nil
}

// Name implements plugin.Plugin.
func (p *localFileOpsPlugin) Name() string { return PluginName }

// Init registers the subset of tools permitted by the config. Disabled tools
// are simply not registered so the LLM never sees them; this is cleaner than
// registering a handler that always returns a denial error.
//
// Returns an error if write is enabled but allowed_roots is empty (which
// would mean writes with no restriction -- rejected to fail closed).
func (p *localFileOpsPlugin) Init(api plugin.PluginAPI) error {
	if !p.cfg.Enabled {
		logger.Info("[local-fileops] plugin disabled via config")
		return nil
	}

	// Build read sandbox (always built when plugin is enabled; read tools
	// may still be individually disabled below).
	p.readSandbox = &fileops.Sandbox{
		ReadAllowedRoots: p.cfg.Read.AllowedRoots,
		DenyPathsExact:   p.cfg.DenyExact,
		DenyPathsPrefix:  p.cfg.DenyPrefix,
		MaxReadBytes:     p.cfg.Read.MaxBytes,
	}

	// Build write sandbox only when write is enabled.
	if p.cfg.Write.Enabled {
		if len(p.cfg.Write.AllowedRoots) == 0 {
			return fmt.Errorf("local-fileops: write.enabled=true requires write.allowed_roots to be non-empty")
		}
		p.writeSandbox = &fileops.Sandbox{
			WriteEnabled:      true,
			WriteAllowedRoots: p.cfg.Write.AllowedRoots,
			ReadAllowedRoots:  p.cfg.Write.AllowedRoots,
			DenyPathsExact:    p.cfg.DenyExact,
			DenyPathsPrefix:   p.cfg.DenyPrefix,
		}
	}

	if p.cfg.Read.Enabled {
		api.RegisterTool(plugin.ToolDefinition{
			Name: "read_file",
			Description: "Read a file from the Hivemind local filesystem with pagination support. " +
				"Returns numbered lines (format: `LINE|CONTENT`) plus metadata. " +
				"Use offset/limit for large files; binary files return metadata only.",
			Parameters: []plugin.ParameterDef{
				{Name: "path", Type: "string", Description: "Absolute or relative file path", Required: true},
				{Name: "offset", Type: "number", Description: "1-indexed line to start from (default 1)"},
				{Name: "limit", Type: "number", Description: "Max lines to return (default 500, cap 2000)"},
			},
			Handler:    p.handleReadFile,
			Category:   "file",
			IsReadOnly: true,
		})
		api.RegisterTool(plugin.ToolDefinition{
			Name: "search_files",
			Description: "Search file contents (regex) or find files by name (glob) on the Hivemind local filesystem. " +
				"Output modes: 'content' (lines + line numbers), 'files_only' (dedup paths), 'count' (per-file match counts). " +
				"Skips .git/node_modules/vendor and binary files automatically.",
			Parameters: []plugin.ParameterDef{
				{Name: "pattern", Type: "string", Description: "Regex (content mode) or glob (files mode)", Required: true},
				{Name: "target", Type: "string", Description: "'content' (default) or 'files'"},
				{Name: "path", Type: "string", Description: "Root directory (default '.')"},
				{Name: "file_glob", Type: "string", Description: "Filter files by glob in content mode"},
				{Name: "limit", Type: "number", Description: "Max results (default 50)"},
				{Name: "offset", Type: "number", Description: "Skip first N results for pagination"},
				{Name: "output_mode", Type: "string", Description: "'content' | 'files_only' | 'count'"},
				{Name: "context", Type: "number", Description: "Context lines around each match"},
			},
			Handler:    p.handleSearchFiles,
			Category:   "file",
			IsReadOnly: true,
		})
	}

	if p.cfg.Write.Enabled {
		api.RegisterTool(plugin.ToolDefinition{
			Name: "write_file",
			Description: "Write content to a file on the Hivemind local filesystem " +
				"(creates parent directories, overwrites existing). Restricted to allowed_roots.",
			Parameters: []plugin.ParameterDef{
				{Name: "path", Type: "string", Description: "Target file path", Required: true},
				{Name: "content", Type: "string", Description: "New file content", Required: true},
			},
			Handler:  p.handleWriteFile,
			Category: "file",
		})
		api.RegisterTool(plugin.ToolDefinition{
			Name: "patch",
			Description: "Edit a file on the Hivemind local filesystem via find-and-replace with fuzzy matching. " +
				"The `old_string` must be unique in the file unless `replace_all=true`. " +
				"Returns a unified diff on success.",
			Parameters: []plugin.ParameterDef{
				{Name: "mode", Type: "string", Description: "'replace' (MVP). 'patch' (V4A) reserved for Phase 2."},
				{Name: "path", Type: "string", Description: "File path (required for replace mode)"},
				{Name: "old_string", Type: "string", Description: "Text to find (required for replace mode)"},
				{Name: "new_string", Type: "string", Description: "Replacement text (may be empty to delete)"},
				{Name: "replace_all", Type: "boolean", Description: "Replace every occurrence instead of requiring uniqueness"},
			},
			Handler:  p.handlePatch,
			Category: "file",
		})
	}

	logger.Info("[local-fileops] initialized (read=%v, write=%v)", p.cfg.Read.Enabled, p.cfg.Write.Enabled)
	return nil
}

// Start / Stop implement LifecyclePlugin for symmetry with peer plugins.
// This plugin has no background state, so both are no-ops.
func (p *localFileOpsPlugin) Start(_ context.Context) error { return nil }
func (p *localFileOpsPlugin) Stop(_ context.Context) error  { return nil }

// Compile-time interface checks.
var (
	_ plugin.Plugin          = (*localFileOpsPlugin)(nil)
	_ plugin.InitPlugin      = (*localFileOpsPlugin)(nil)
	_ plugin.LifecyclePlugin = (*localFileOpsPlugin)(nil)
)
