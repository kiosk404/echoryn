// Package fileops implements the file_read / file_write / file_patch /
// file_search skills executed by Golem's TaskExecutor. Handlers decode
// payload bytes, delegate to pkg/fileops, and return JSON-encoded results.
package fileops

// Skill name constants. These are the canonical skill names that Hivemind
// must use when dispatching tasks via cluster_dispatch_task or the typed
// golem_* tools. Must stay in sync with internal/golem/handler/control_handler.go.
const (
	SkillFileRead   = "file_read"
	SkillFileWrite  = "file_write"
	SkillFilePatch  = "file_patch"
	SkillFileSearch = "file_search"
)

// ReadPayload is the JSON payload shape for file_read tasks.
type ReadPayload struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// WritePayload is the JSON payload shape for file_write tasks.
type WritePayload struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// PatchPayload is the JSON payload shape for file_patch tasks.
type PatchPayload struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

// SearchPayload is the JSON payload shape for file_search tasks.
type SearchPayload struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Target     string `json:"target"`
	FileGlob   string `json:"file_glob"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	OutputMode string `json:"output_mode"`
	Context    int    `json:"context"`
}
