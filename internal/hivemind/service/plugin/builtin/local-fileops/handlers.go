package localfileops

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/pkg/fileops"
)

// handleReadFile implements the read_file tool. Params:
//
//	path    string   (required)
//	offset  number   (default 1, 1-indexed)
//	limit   number   (default 500, clamp 2000)
func (p *localFileOpsPlugin) handleReadFile(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("parameter 'path' is required")
	}
	offset := 1
	if v, ok := params["offset"].(float64); ok {
		offset = int(v)
	}
	limit := 500
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}
	return fileops.ReadFile(p.readSandbox, path, offset, limit)
}

// handleWriteFile implements the write_file tool. Params:
//
//	path     string (required)
//	content  string (required)
func (p *localFileOpsPlugin) handleWriteFile(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)
	if path == "" {
		return nil, fmt.Errorf("parameter 'path' is required")
	}
	return fileops.WriteFile(p.writeSandbox, path, content)
}

// handlePatch implements the patch tool (replace mode only in MVP). Params:
//
//	mode         string   "replace" (default; "patch" reserved for V4A in Phase 2)
//	path         string   (required for replace mode)
//	old_string   string   (required for replace mode)
//	new_string   string
//	replace_all  bool
func (p *localFileOpsPlugin) handlePatch(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	mode, _ := params["mode"].(string)
	if mode == "" {
		mode = "replace"
	}
	if mode != "replace" {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("patch mode %q is not supported in MVP (only 'replace')", mode),
		}, nil
	}
	path, _ := params["path"].(string)
	oldStr, _ := params["old_string"].(string)
	newStr, _ := params["new_string"].(string)
	replaceAll, _ := params["replace_all"].(bool)
	if path == "" || oldStr == "" {
		return nil, fmt.Errorf("parameters 'path' and 'old_string' are required for replace mode")
	}
	return fileops.PatchReplace(p.writeSandbox, path, oldStr, newStr, replaceAll)
}

// handleSearchFiles implements the search_files tool. Full parameter set
// mirrors fileops.SearchOptions; see that struct for semantics.
func (p *localFileOpsPlugin) handleSearchFiles(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return nil, fmt.Errorf("parameter 'pattern' is required")
	}
	opts := fileops.SearchOptions{Pattern: pattern}
	if v, ok := params["target"].(string); ok {
		opts.Target = v
	}
	if v, ok := params["path"].(string); ok && v != "" {
		opts.Path = v
	} else {
		opts.Path = "."
	}
	if v, ok := params["file_glob"].(string); ok {
		opts.FileGlob = v
	}
	if v, ok := params["limit"].(float64); ok {
		opts.Limit = int(v)
	}
	if v, ok := params["offset"].(float64); ok {
		opts.Offset = int(v)
	}
	if v, ok := params["output_mode"].(string); ok {
		opts.OutputMode = v
	}
	if v, ok := params["context"].(float64); ok {
		opts.Context = int(v)
	}
	return fileops.Search(p.readSandbox, opts)
}
