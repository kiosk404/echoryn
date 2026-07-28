package fileops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestSandbox(t *testing.T) (*Sandbox, string) {
	t.Helper()
	dir := t.TempDir()
	return &Sandbox{
		WriteEnabled:      true,
		WriteAllowedRoots: []string{dir},
		ReadAllowedRoots:  []string{dir},
	}, dir
}

func TestReadFileBasic(t *testing.T) {
	sb, dir := newTestSandbox(t)
	p := filepath.Join(dir, "a.txt")
	os.WriteFile(p, []byte("line1\nline2\nline3\n"), 0o644)
	r, err := ReadFile(sb, p, 1, 500)
	if err != nil {
		t.Fatal(err)
	}
	if r.TotalLines != 3 {
		t.Errorf("TotalLines=%d want 3", r.TotalLines)
	}
	if !strings.Contains(r.Content, "line1") {
		t.Errorf("missing line1")
	}
}

func TestReadFilePagination(t *testing.T) {
	sb, dir := newTestSandbox(t)
	p := filepath.Join(dir, "b.txt")
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "line")
	}
	os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644)
	r, _ := ReadFile(sb, p, 5, 3)
	if strings.Count(r.Content, "\n") > 3 {
		t.Errorf("over limit: %q", r.Content)
	}
}

func TestReadFileDeniedByDenyList(t *testing.T) {
	r, _ := ReadFile(&Sandbox{}, "/etc/passwd", 1, 500)
	if r.Error == "" {
		t.Error("expected error")
	}
}

func TestReadFileBinaryDetection(t *testing.T) {
	sb, dir := newTestSandbox(t)
	p := filepath.Join(dir, "blob.bin")
	os.WriteFile(p, []byte{0, 0, 0, 0, 0, 1, 2, 3}, 0o644)
	r, _ := ReadFile(sb, p, 1, 500)
	if !r.IsBinary {
		t.Error("expected IsBinary")
	}
	if r.Content != "" {
		t.Error("binary must not return content")
	}
}

func TestReadFileNotFoundReturnsSimilar(t *testing.T) {
	sb, dir := newTestSandbox(t)
	os.WriteFile(filepath.Join(dir, "apple.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "apricot.txt"), []byte("x"), 0o644)
	r, _ := ReadFile(sb, filepath.Join(dir, "appl.txt"), 1, 500)
	if r.Error == "" {
		t.Error("expected error")
	}
	if len(r.SimilarFiles) == 0 {
		t.Error("expected SimilarFiles")
	}
}
