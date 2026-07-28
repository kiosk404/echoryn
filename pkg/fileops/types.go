package fileops

// ReadResult is the result of ReadFile / ReadFileRaw.
type ReadResult struct {
	Content      string   `json:"content,omitempty"`
	TotalLines   int      `json:"total_lines"`
	FileSize     int64    `json:"file_size"`
	Truncated    bool     `json:"truncated,omitempty"`
	Hint         string   `json:"hint,omitempty"`
	IsBinary     bool     `json:"is_binary,omitempty"`
	IsImage      bool     `json:"is_image,omitempty"`
	Base64       string   `json:"base64_content,omitempty"`
	MimeType     string   `json:"mime_type,omitempty"`
	Error        string   `json:"error,omitempty"`
	SimilarFiles []string `json:"similar_files,omitempty"`
}

// WriteResult is the result of WriteFile.
type WriteResult struct {
	BytesWritten int    `json:"bytes_written"`
	DirsCreated  bool   `json:"dirs_created,omitempty"`
	Error        string `json:"error,omitempty"`
	Warning      string `json:"warning,omitempty"`
}

// PatchResult is the result of PatchReplace (and future PatchV4A).
type PatchResult struct {
	Success       bool     `json:"success"`
	Diff          string   `json:"diff,omitempty"`
	FilesModified []string `json:"files_modified,omitempty"`
	FilesCreated  []string `json:"files_created,omitempty"`
	FilesDeleted  []string `json:"files_deleted,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// SearchMatch is a single match inside search results.
type SearchMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// SearchResult is the result of Search.
type SearchResult struct {
	Matches    []SearchMatch  `json:"matches,omitempty"`
	Files      []string       `json:"files,omitempty"`
	Counts     map[string]int `json:"counts,omitempty"`
	TotalCount int            `json:"total_count"`
	Truncated  bool           `json:"truncated,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// SearchOptions parametrizes Search.
type SearchOptions struct {
	Pattern    string // regex for content mode, glob for files mode
	Path       string // root dir (default ".")
	Target     string // "content" | "files"
	FileGlob   string // filter within content mode
	Limit      int    // default 50
	Offset     int    // default 0
	OutputMode string // "content" | "files_only" | "count"
	Context    int    // before/after lines (content mode)
}
