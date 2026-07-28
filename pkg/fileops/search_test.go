package fileops

import (
	"os"
	"path/filepath"
	"testing"
)

func setupSearchTree(t *testing.T) (*Sandbox, string) {
	sb, dir := newTestSandbox(t)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc HelloAlpha(){}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc HelloBeta(){}\n"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("lorem Hello\nipsum"), 0o644)
	return sb, dir
}

func TestSearchContentMode(t *testing.T) {
	sb, dir := setupSearchTree(t)
	r, err := Search(sb, SearchOptions{Pattern: "Hello", Path: dir, Target: "content"})
	if err != nil {
		t.Fatal(err)
	}
	if r.TotalCount != 3 {
		t.Errorf("TotalCount=%d", r.TotalCount)
	}
}

func TestSearchContentModeWithFileGlob(t *testing.T) {
	sb, dir := setupSearchTree(t)
	r, _ := Search(sb, SearchOptions{Pattern: "Hello", Path: dir, Target: "content", FileGlob: "*.go"})
	if r.TotalCount != 2 {
		t.Errorf("got %d", r.TotalCount)
	}
}

func TestSearchFilesMode(t *testing.T) {
	sb, dir := setupSearchTree(t)
	r, _ := Search(sb, SearchOptions{Pattern: "*.go", Path: dir, Target: "files"})
	if r.TotalCount != 2 {
		t.Errorf("got %d", r.TotalCount)
	}
}

func TestSearchOutputFilesOnly(t *testing.T) {
	sb, dir := setupSearchTree(t)
	r, _ := Search(sb, SearchOptions{Pattern: "Hello", Path: dir, Target: "content", OutputMode: "files_only"})
	// Three files contain "Hello": a.go, b.go, sub/c.txt.
	if len(r.Files) != 3 {
		t.Errorf("got %v", r.Files)
	}
}

func TestSearchOutputCount(t *testing.T) {
	sb, dir := setupSearchTree(t)
	r, _ := Search(sb, SearchOptions{Pattern: "Hello", Path: dir, Target: "content", OutputMode: "count"})
	if len(r.Counts) != 3 {
		t.Errorf("got %v", r.Counts)
	}
}

func TestSearchDeniesRootOutsideSandbox(t *testing.T) {
	sb := &Sandbox{ReadAllowedRoots: []string{"/some/other/root"}}
	r, _ := Search(sb, SearchOptions{Pattern: ".", Path: "/tmp", Target: "content"})
	if r.Error == "" {
		t.Error("expected denial")
	}
}
