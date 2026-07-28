package fileops

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// Search performs a ripgrep-like search of either file contents (regex) or
// filenames (glob). The behavior is controlled by opts.Target:
//
//   - "content" (default): opts.Pattern is compiled as a regex and matched
//     line-by-line against text files under opts.Path. opts.FileGlob, when
//     set, filters which files are scanned.
//   - "files": opts.Pattern is treated as a glob (filepath.Match) and tested
//     against every filename under opts.Path.
//
// Common directories ".git", "node_modules", "vendor" are skipped. Binary
// files (by extension) are skipped in content mode.
//
// Output shape is selected via opts.OutputMode: "content" (default) returns
// a flat list of matches; "files_only" returns deduplicated file paths;
// "count" returns a map of path -> match count.
//
// Sandbox.CheckRead is invoked on opts.Path; if it fails, the error is
// encoded in r.Error (not returned), consistent with the rest of the package.
func Search(sb *Sandbox, opts SearchOptions) (*SearchResult, error) {
	r := &SearchResult{Counts: make(map[string]int)}
	root := opts.Path
	if root == "" {
		root = "."
	}
	if err := sb.CheckRead(root); err != nil {
		r.Error = err.Error()
		return r, nil
	}
	resolvedRoot, _ := resolvePath(root)
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if max := sb.EffectiveMaxSearchResults(); limit > max {
		limit = max
	}
	target := opts.Target
	if target == "" {
		target = "content"
	}
	outMode := opts.OutputMode
	if outMode == "" {
		outMode = "content"
	}

	if target == "files" {
		return searchFilesByGlob(resolvedRoot, opts.Pattern, opts.Offset, limit)
	}

	re, err := regexp.Compile(opts.Pattern)
	if err != nil {
		r.Error = fmt.Sprintf("invalid regex: %v", err)
		return r, nil
	}

	var all []SearchMatch
	seen := make(map[string]int)

	_ = filepath.WalkDir(resolvedRoot, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			n := d.Name()
			if n == ".git" || n == "node_modules" || n == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if IsBinary(path, nil) {
			return nil
		}
		if opts.FileGlob != "" {
			m, _ := filepath.Match(opts.FileGlob, d.Name())
			if !m {
				return nil
			}
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		ln := 0
		for sc.Scan() {
			ln++
			line := sc.Text()
			if re.MatchString(line) {
				all = append(all, SearchMatch{Path: path, Line: ln, Content: line})
				seen[path]++
			}
		}
		return nil
	})

	r.TotalCount = len(all)
	switch outMode {
	case "files_only":
		files := make([]string, 0, len(seen))
		for f := range seen {
			files = append(files, f)
		}
		sort.Strings(files)
		if opts.Offset < len(files) {
			files = files[opts.Offset:]
		}
		if len(files) > limit {
			files = files[:limit]
			r.Truncated = true
		}
		r.Files = files
	case "count":
		r.Counts = seen
	default:
		start := opts.Offset
		if start > len(all) {
			start = len(all)
		}
		end := start + limit
		if end > len(all) {
			end = len(all)
		}
		r.Matches = all[start:end]
		r.Truncated = end < len(all)
	}
	return r, nil
}

// searchFilesByGlob handles the Target="files" mode: walks root and collects
// every file whose basename matches the glob. Results are sorted by modtime
// (newest first) -- useful for "find the most recent config file" patterns.
func searchFilesByGlob(root, pattern string, offset, limit int) (*SearchResult, error) {
	r := &SearchResult{}
	var paths []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		matched, _ := filepath.Match(pattern, d.Name())
		if matched {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Slice(paths, func(i, j int) bool {
		si, _ := os.Stat(paths[i])
		sj, _ := os.Stat(paths[j])
		if si == nil || sj == nil {
			return paths[i] < paths[j]
		}
		return si.ModTime().After(sj.ModTime())
	})
	r.TotalCount = len(paths)
	if offset < len(paths) {
		paths = paths[offset:]
	} else {
		paths = nil
	}
	if len(paths) > limit {
		paths = paths[:limit]
		r.Truncated = true
	}
	r.Files = paths
	return r, nil
}
