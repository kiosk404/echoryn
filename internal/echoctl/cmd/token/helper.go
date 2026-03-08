package token

import (
	"os"
	"strings"

	"github.com/kiosk404/echoryn/pkg/paths"
)

// readAdminTokenFromFile attempts to read the admin token from multiple locations.
// Priority: 1) local .echoryn/credentials/admin_token 2) ~/.echoryn/credentials/admin_token
func readAdminTokenFromFile() string {
	// Check local project directory firest (for development mode)
	localPaths := []string{
		".echoryn/credentials/admin_token",
		paths.ResolveAdminTokenPath(),
	}
	for _, p := range localPaths {
		if data, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}
