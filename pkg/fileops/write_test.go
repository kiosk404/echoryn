package fileops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileCreatesDirs(t *testing.T) {
	sb, dir := newTestSandbox(t)
	target := filepath.Join(dir, "sub1", "sub2", "a.txt")
	r, err := WriteFile(sb, target, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if r.Error != "" {
		t.Fatalf("r.Error: %s", r.Error)
	}
	if !r.DirsCreated {
		t.Error("DirsCreated should be true")
	}
	if r.BytesWritten != 5 {
		t.Errorf("BytesWritten=%d", r.BytesWritten)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "hello" {
		t.Errorf("content %q", string(got))
	}
}

func TestWriteFileDeniedWhenWriteDisabled(t *testing.T) {
	dir := t.TempDir()
	sb := &Sandbox{WriteEnabled: false, ReadAllowedRoots: []string{dir}}
	r, _ := WriteFile(sb, filepath.Join(dir, "x"), "content")
	if r.Error == "" {
		t.Error("expected denial")
	}
}

func TestWriteFileDeniedOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	sb := &Sandbox{WriteEnabled: true, WriteAllowedRoots: []string{dir}}
	r, _ := WriteFile(sb, "/tmp/out-of-root-xyz.txt", "x")
	if r.Error == "" {
		t.Error("expected denial")
	}
}
