package fileops

import (
	"fmt"
	"os"
)

// PatchReplace replaces occurrences of `oldStr` with `newStr` in the file at
// `path`. When replaceAll is false, the match must be unique (exact
// occurrence count == 1); otherwise an error is returned directing the
// caller to either add more context or set replace_all=true.
//
// Matching strategy:
//   - replace_all=true: exact matches only (FindAllExact), all replaced.
//   - replace_all=false, single exact match: replaced.
//   - replace_all=false, no exact match: fall back to FindBestMatch's fuzzy
//     ladder (trimmed whitespace, normalized line endings, indent-normalized).
//
// Returns PatchResult with populated Diff on success, or Error on failure.
// Never returns a non-nil error -- errors are always encoded in r.Error so
// the caller (tool handler) can surface them to the LLM as tool output.
func PatchReplace(sb *Sandbox, path, oldStr, newStr string, replaceAll bool) (*PatchResult, error) {
	r := &PatchResult{}
	if err := sb.CheckWrite(path); err != nil {
		r.Error = err.Error()
		return r, nil
	}
	if oldStr == "" {
		r.Error = "old_string must not be empty"
		return r, nil
	}
	resolved, _ := resolvePath(path)
	origBytes, err := os.ReadFile(resolved)
	if err != nil {
		r.Error = fmt.Sprintf("read: %v", err)
		return r, nil
	}
	orig := string(origBytes)

	var newContent string
	if replaceAll {
		ranges, err := FindAllExact(orig, oldStr)
		if err != nil {
			r.Error = fmt.Sprintf("old_string not found: %v", err)
			return r, nil
		}
		// Replace from the end to preserve earlier offsets.
		newContent = orig
		for i := len(ranges) - 1; i >= 0; i-- {
			rg := ranges[i]
			newContent = newContent[:rg.Start] + newStr + newContent[rg.End:]
		}
	} else {
		if ranges, err := FindAllExact(orig, oldStr); err == nil {
			if len(ranges) > 1 {
				r.Error = fmt.Sprintf("old_string matches %d locations; set replace_all=true or add context", len(ranges))
				return r, nil
			}
			rg := ranges[0]
			newContent = orig[:rg.Start] + newStr + orig[rg.End:]
		} else {
			idx, length, ferr := FindBestMatch(orig, oldStr)
			if ferr != nil {
				r.Error = fmt.Sprintf("old_string not found: %v", ferr)
				return r, nil
			}
			newContent = orig[:idx] + newStr + orig[idx+length:]
		}
	}

	if err := os.WriteFile(resolved, []byte(newContent), 0o644); err != nil {
		r.Error = fmt.Sprintf("write: %v", err)
		return r, nil
	}
	r.Success = true
	r.FilesModified = []string{resolved}
	r.Diff = UnifiedDiff(orig, newContent, path, path)
	return r, nil
}
