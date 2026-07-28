package fileops

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile writes content to path, creating parent directories as needed.
// Refuses when the Sandbox disallows.
func WriteFile(sb *Sandbox, path, content string) (*WriteResult, error) {
	r := &WriteResult{}
	if err := sb.CheckWrite(path); err != nil {
		r.Error = err.Error()
		return r, nil
	}
	resolved, err := resolveForWrite(path)
	if err != nil {
		r.Error = fmt.Sprintf("resolve: %v", err)
		return r, nil
	}
	dir := filepath.Dir(resolved)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			r.Error = fmt.Sprintf("mkdir: %v", err)
			return r, nil
		}
		r.DirsCreated = true
	}
	if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
		r.Error = fmt.Sprintf("write: %v", err)
		return r, nil
	}
	r.BytesWritten = len(content)
	return r, nil
}
