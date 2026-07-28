package fileops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchReplaceExact(t *testing.T) {
	sb, dir := newTestSandbox(t)
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("hello world\nfoo bar\n"), 0o644)
	r, err := PatchReplace(sb, p, "world", "golang", false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Success {
		t.Fatalf("not success: %s", r.Error)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "hello golang") {
		t.Errorf("got %s", b)
	}
	if r.Diff == "" {
		t.Error("diff empty")
	}
}

func TestPatchReplaceNonUniqueWithoutFlag(t *testing.T) {
	sb, dir := newTestSandbox(t)
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("a foo b foo c"), 0o644)
	r, _ := PatchReplace(sb, p, "foo", "bar", false)
	if r.Success {
		t.Error("should fail on non-unique")
	}
}

func TestPatchReplaceAll(t *testing.T) {
	sb, dir := newTestSandbox(t)
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("a foo b foo c"), 0o644)
	r, _ := PatchReplace(sb, p, "foo", "bar", true)
	if !r.Success {
		t.Fatal(r.Error)
	}
	b, _ := os.ReadFile(p)
	if strings.Count(string(b), "bar") != 2 {
		t.Errorf("got %s", b)
	}
}

func TestPatchReplaceNoMatch(t *testing.T) {
	sb, dir := newTestSandbox(t)
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("hello"), 0o644)
	r, _ := PatchReplace(sb, p, "xxx", "y", false)
	if r.Success {
		t.Error("should fail")
	}
}
