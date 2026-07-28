package fileops

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultReadLimit = 500
	maxReadLimit     = 2000
	maxLineLength    = 2000
)

// ReadFile reads a file returning numbered lines with pagination support.
// offset is 1-indexed; limit is clamped to maxReadLimit.
func ReadFile(sb *Sandbox, path string, offset, limit int) (*ReadResult, error) {
	r := &ReadResult{}
	if offset < 1 {
		offset = 1
	}
	if limit <= 0 {
		limit = defaultReadLimit
	}
	if limit > maxReadLimit {
		limit = maxReadLimit
	}
	if err := sb.CheckRead(path); err != nil {
		r.Error = err.Error()
		return r, nil
	}
	resolved, _ := resolvePath(path)
	info, err := os.Stat(resolved)
	if err != nil {
		r.Error = fmt.Sprintf("stat: %v", err)
		r.SimilarFiles = suggestSimilarFiles(resolved)
		return r, nil
	}
	r.FileSize = info.Size()
	if r.FileSize > sb.EffectiveMaxReadBytes() {
		r.Error = fmt.Sprintf("file size %d exceeds MaxReadBytes", r.FileSize)
		r.Hint = "use pagination (offset, limit) on a narrower range"
		return r, nil
	}
	if IsBinary(resolved, nil) {
		r.IsBinary = true
		if IsImage(resolved) {
			r.IsImage = true
		}
		r.Hint = "binary file; content not returned"
		return r, nil
	}
	f, err := os.Open(resolved)
	if err != nil {
		r.Error = fmt.Sprintf("open: %v", err)
		return r, nil
	}
	defer f.Close()
	head := make([]byte, 1024)
	n, _ := f.Read(head)
	if IsBinary(resolved, head[:n]) {
		r.IsBinary = true
		r.Hint = "binary content detected by sampling"
		return r, nil
	}
	if _, err := f.Seek(0, 0); err != nil {
		r.Error = fmt.Sprintf("seek: %v", err)
		return r, nil
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var out strings.Builder
	lineNo, emitted := 0, 0
	for sc.Scan() {
		lineNo++
		if lineNo < offset {
			continue
		}
		if emitted >= limit {
			continue
		}
		line := sc.Text()
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "... [truncated]"
		}
		fmt.Fprintf(&out, "%6d|%s\n", lineNo, line)
		emitted++
	}
	if err := sc.Err(); err != nil {
		r.Error = fmt.Sprintf("scan: %v", err)
		return r, nil
	}
	r.TotalLines = lineNo
	r.Content = out.String()
	r.Truncated = emitted < (lineNo - offset + 1)
	return r, nil
}

// ReadFileRaw returns the full file content without pagination or line numbers.
// Still respects Sandbox.MaxReadBytes.
func ReadFileRaw(sb *Sandbox, path string) (*ReadResult, error) {
	r := &ReadResult{}
	if err := sb.CheckRead(path); err != nil {
		r.Error = err.Error()
		return r, nil
	}
	resolved, _ := resolvePath(path)
	info, err := os.Stat(resolved)
	if err != nil {
		r.Error = fmt.Sprintf("stat: %v", err)
		return r, nil
	}
	r.FileSize = info.Size()
	if r.FileSize > sb.EffectiveMaxReadBytes() {
		r.Error = "file too large"
		return r, nil
	}
	b, err := os.ReadFile(resolved)
	if err != nil {
		r.Error = fmt.Sprintf("read: %v", err)
		return r, nil
	}
	if IsBinary(resolved, b) {
		r.IsBinary = true
		return r, nil
	}
	r.Content = string(b)
	r.TotalLines = strings.Count(r.Content, "\n")
	return r, nil
}

// suggestSimilarFiles returns up to 3 paths in the same directory whose
// basename is similar to the requested base (shared prefix of >=3 chars or
// one contains the other). Used to provide friendly "did you mean?" hints
// on not-found errors.
func suggestSimilarFiles(path string) []string {
	dir := filepath.Dir(path)
	base := strings.ToLower(filepath.Base(path))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	const minPrefix = 3
	var out []string
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if isSimilarName(name, base, minPrefix) {
			out = append(out, filepath.Join(dir, e.Name()))
			if len(out) >= 3 {
				break
			}
		}
	}
	return out
}

// isSimilarName returns true if a and b are "similar enough" that a would
// likely be the intended target when b was typed. Heuristics:
//  1. Substring match in either direction ("appconfig" ~ "config")
//  2. Shared prefix of >= minPrefix chars ("appl.txt" ~ "apple.txt")
func isSimilarName(a, b string, minPrefix int) bool {
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}
	prefix := 0
	for i := 0; i < len(a) && i < len(b) && a[i] == b[i]; i++ {
		prefix++
	}
	return prefix >= minPrefix
}
