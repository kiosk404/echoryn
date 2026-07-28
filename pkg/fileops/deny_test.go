package fileops

import (
	"strings"
	"testing"
)

func TestBuiltinDenyExactContainsPasswd(t *testing.T) {
	for _, p := range BuiltinDenyExact {
		if p == "/etc/passwd" {
			return
		}
	}
	t.Error("/etc/passwd missing from BuiltinDenyExact")
}

func TestBuiltinDenyPrefixContainsSSH(t *testing.T) {
	for _, p := range BuiltinDenyPrefix {
		if strings.Contains(p, ".ssh") {
			return
		}
	}
	t.Error("~/.ssh prefix missing from BuiltinDenyPrefix")
}

func TestBuiltinDenyPrefixEndsWithSeparator(t *testing.T) {
	for _, p := range BuiltinDenyPrefix {
		if !strings.HasSuffix(p, "/") {
			t.Errorf("deny prefix %q must end with '/'", p)
		}
	}
}
