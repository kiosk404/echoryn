package fileops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSandboxCheckReadDeniesBuiltinExact(t *testing.T) {
	if err := (&Sandbox{}).CheckRead("/etc/passwd"); err == nil {
		t.Error("expected /etc/passwd denied")
	}
}

func TestSandboxCheckReadAllowsWhenNoRoots(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	os.WriteFile(f, []byte("x"), 0o644)
	if err := (&Sandbox{}).CheckRead(f); err != nil {
		t.Errorf("no roots should permit: %v", err)
	}
}

func TestSandboxCheckReadEnforcesRoots(t *testing.T) {
	in := t.TempDir()
	inside := filepath.Join(in, "ok.txt")
	os.WriteFile(inside, []byte("x"), 0o644)
	out := t.TempDir()
	outside := filepath.Join(out, "no.txt")
	os.WriteFile(outside, []byte("x"), 0o644)
	sb := &Sandbox{ReadAllowedRoots: []string{in}}
	if err := sb.CheckRead(inside); err != nil {
		t.Errorf("inside fail: %v", err)
	}
	if err := sb.CheckRead(outside); err == nil {
		t.Error("outside should fail")
	}
}

func TestSandboxCheckWriteRequiresWriteEnabled(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	if err := (&Sandbox{WriteEnabled: false}).CheckWrite(f); err == nil {
		t.Error("WriteEnabled=false must deny")
	}
	if err := (&Sandbox{WriteEnabled: true, WriteAllowedRoots: []string{dir}}).CheckWrite(f); err != nil {
		t.Errorf("should allow: %v", err)
	}
}

func TestSandboxResolvesSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	os.WriteFile(real, []byte("x"), 0o644)
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlink unsupported")
	}
	if err := (&Sandbox{ReadAllowedRoots: []string{dir}}).CheckRead(link); err != nil {
		t.Errorf("symlink inside root should pass: %v", err)
	}
}
