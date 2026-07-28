package fileops

import (
	"os"
	"path/filepath"
	"strings"
)

// BuiltinDenyExact lists absolute file paths that must never be touched by
// any file operation, regardless of sandbox configuration.
var BuiltinDenyExact []string

// BuiltinDenyPrefix lists absolute path prefixes (ending in the OS path
// separator) that must never be touched by any file operation.
var BuiltinDenyPrefix []string

func init() {
	home, _ := os.UserHomeDir()

	exact := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/etc/sudoers",
	}
	if home != "" {
		for _, p := range []string{
			".ssh/authorized_keys", ".ssh/id_rsa", ".ssh/id_ed25519",
			".ssh/id_ecdsa", ".ssh/id_dsa", ".ssh/config",
			".netrc", ".pgpass", ".npmrc", ".pypirc",
		} {
			exact = append(exact, filepath.Join(home, p))
		}
	}
	BuiltinDenyExact = realpaths(exact)

	prefix := []string{
		"/etc/sudoers.d/",
		"/etc/systemd/",
	}
	if home != "" {
		for _, p := range []string{".ssh", ".aws", ".gnupg", ".kube", ".docker", ".azure"} {
			prefix = append(prefix, filepath.Join(home, p)+string(filepath.Separator))
		}
	}
	BuiltinDenyPrefix = realpathPrefixes(prefix)
}

// realpaths resolves each entry via EvalSymlinks so that deny-list comparisons
// work on platforms where system dirs are symlinks (e.g. macOS: /etc -> /private/etc).
// If resolution fails (file may not exist) the original path is kept.
func realpaths(paths []string) []string {
	out := make([]string, 0, len(paths)*2)
	for _, p := range paths {
		seen := map[string]struct{}{p: {}}
		out = append(out, p)
		if real, err := filepath.EvalSymlinks(p); err == nil && real != p {
			if _, ok := seen[real]; !ok {
				out = append(out, real)
			}
		}
		// Also try resolving the parent dir (in case the file itself doesn't exist yet).
		parent := filepath.Dir(p)
		if realParent, err := filepath.EvalSymlinks(parent); err == nil && realParent != parent {
			candidate := filepath.Join(realParent, filepath.Base(p))
			if _, ok := seen[candidate]; !ok {
				out = append(out, candidate)
			}
		}
	}
	return out
}

// realpathPrefixes is the prefix-variant of realpaths: preserves the trailing
// separator on each resolved entry so prefix matching still works.
func realpathPrefixes(prefixes []string) []string {
	out := make([]string, 0, len(prefixes)*2)
	sep := string(filepath.Separator)
	for _, p := range prefixes {
		out = append(out, p)
		trimmed := strings.TrimRight(p, sep)
		if real, err := filepath.EvalSymlinks(trimmed); err == nil && real != trimmed {
			out = append(out, real+sep)
		}
	}
	return out
}
