package fileops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sandbox enforces file-operation boundaries for a single caller (Hivemind
// local-fileops plugin or Golem fileops skill). A zero-value Sandbox denies
// all writes and relies solely on the builtin deny list for reads.
type Sandbox struct {
	ReadAllowedRoots  []string
	WriteEnabled      bool
	WriteAllowedRoots []string
	DenyPathsExact    []string
	DenyPathsPrefix   []string
	MaxReadBytes      int64
	MaxSearchResults  int
}

const (
	DefaultMaxReadBytes     int64 = 5 * 1024 * 1024
	DefaultMaxSearchResults       = 500
)

// EffectiveMaxReadBytes returns MaxReadBytes or the default.
func (s *Sandbox) EffectiveMaxReadBytes() int64 {
	if s == nil || s.MaxReadBytes <= 0 {
		return DefaultMaxReadBytes
	}
	return s.MaxReadBytes
}

// EffectiveMaxSearchResults returns MaxSearchResults or the default.
func (s *Sandbox) EffectiveMaxSearchResults() int {
	if s == nil || s.MaxSearchResults <= 0 {
		return DefaultMaxSearchResults
	}
	return s.MaxSearchResults
}

// CheckRead verifies the path is readable under this sandbox.
func (s *Sandbox) CheckRead(path string) error {
	resolved, err := resolvePath(path)
	if err != nil {
		return err
	}
	if s != nil && isDenied(resolved, s.DenyPathsExact, s.DenyPathsPrefix) {
		return fmt.Errorf("path %q denied by sandbox", path)
	}
	if isDenied(resolved, BuiltinDenyExact, BuiltinDenyPrefix) {
		return fmt.Errorf("path %q denied by builtin deny list", path)
	}
	if s != nil && len(s.ReadAllowedRoots) > 0 && !withinAnyRoot(resolved, s.ReadAllowedRoots) {
		return fmt.Errorf("path %q outside allowed read roots", path)
	}
	return nil
}

// CheckWrite verifies the path is writable under this sandbox.
func (s *Sandbox) CheckWrite(path string) error {
	if s == nil || !s.WriteEnabled {
		return fmt.Errorf("writes are disabled in this sandbox")
	}
	resolved, err := resolveForWrite(path)
	if err != nil {
		return err
	}
	if isDenied(resolved, s.DenyPathsExact, s.DenyPathsPrefix) {
		return fmt.Errorf("path %q denied by sandbox", path)
	}
	if isDenied(resolved, BuiltinDenyExact, BuiltinDenyPrefix) {
		return fmt.Errorf("path %q denied by builtin deny list", path)
	}
	if len(s.WriteAllowedRoots) > 0 && !withinAnyRoot(resolved, s.WriteAllowedRoots) {
		return fmt.Errorf("path %q outside allowed write roots", path)
	}
	return nil
}

func resolvePath(p string) (string, error) {
	p = os.ExpandEnv(p)
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	// Fast path: whole path exists, resolve all symlinks.
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	// Slow path: path doesn't exist. Walk up until we find an existing
	// ancestor, resolve that, and re-append the unresolved suffix. This is
	// essential on macOS where /var and /tmp are symlinks -- without it,
	// sandbox root comparisons would fail for any not-yet-existing file
	// inside a symlinked directory.
	return resolveLongestExistingPrefix(abs), nil
}

// resolveLongestExistingPrefix walks up from abs, finds the deepest ancestor
// that exists, EvalSymlinks it, and reattaches the remaining suffix. Returns
// abs unchanged if no existing ancestor is found.
func resolveLongestExistingPrefix(abs string) string {
	suffix := ""
	current := abs
	for {
		if _, err := os.Lstat(current); err == nil {
			if real, err := filepath.EvalSymlinks(current); err == nil {
				if suffix == "" {
					return real
				}
				return filepath.Join(real, suffix)
			}
			return abs
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs
		}
		suffix = filepath.Join(filepath.Base(current), suffix)
		current = parent
	}
}

func resolveForWrite(p string) (string, error) {
	// resolvePath now handles the non-existent-file case via
	// resolveLongestExistingPrefix, so we can delegate.
	return resolvePath(p)
}

func isDenied(resolved string, exact, prefix []string) bool {
	for _, e := range exact {
		if resolved == e {
			return true
		}
	}
	for _, p := range prefix {
		if strings.HasPrefix(resolved+string(filepath.Separator), p) ||
			strings.HasPrefix(resolved, p) {
			return true
		}
	}
	return false
}

func withinAnyRoot(resolved string, roots []string) bool {
	for _, r := range roots {
		realRoot, err := filepath.EvalSymlinks(r)
		if err != nil {
			realRoot, _ = filepath.Abs(r)
		}
		if resolved == realRoot ||
			strings.HasPrefix(resolved, realRoot+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
