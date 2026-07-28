package fileops

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// UnifiedDiff returns a unified diff (3 context lines) between oldContent and
// newContent, labeled with fromFile / toFile paths in the standard a/ b/
// convention. Returns "" on error (which indicates an internal go-difflib
// failure; neither input is user-facing).
func UnifiedDiff(oldContent, newContent, fromFile, toFile string) string {
	d := difflib.UnifiedDiff{
		A:        difflib.SplitLines(oldContent),
		B:        difflib.SplitLines(newContent),
		FromFile: "a/" + fromFile,
		ToFile:   "b/" + toFile,
		Context:  3,
	}
	text, err := difflib.GetUnifiedDiffString(d)
	if err != nil {
		return ""
	}
	return strings.TrimRight(text, "\n")
}
