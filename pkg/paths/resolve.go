// Package paths provides centralized path resolution for the Echoryn runtime.
//
// All paths in the Echoryn system derive from a single "state directory" (~/.echoryn),
// which serves as the canonical root for configuration, session data, memory indices,
// and workspace prompt files.
//
// The system supports two node roles:
//   - Hivemind (control plane): config at ~/.echoryn/hivemind.json
//   - Golem (worker node):      config at ~/.echoryn/golem.json
//
// Design principles (aligned with OpenClaw's config/paths.ts):
//   - resolve* functions are pure path computation (no I/O)
//   - Environment variable overrides take the highest priority
//   - Ensure* functions perform I/O (MkdirAll) and are called once at startup
package paths

import (
	"os"
	"path/filepath"
	"sync"
)

// NodeRole identifies the type of node in the Echoryn cluster.
type NodeRole string

const (
	// RoleHivemind is the control plane / central brain node.
	RoleHivemind NodeRole = "hivemind"
	// RoleGolem is the worker / executor node.
	RoleGolem NodeRole = "golem"
)

// ConfigFileName returns the configuration file name for the given role.
func (r NodeRole) ConfigFileName() string {
	switch r {
	case RoleGolem:
		return golemConfigFileName
	default:
		return hivemindConfigFileName
	}
}

// String returns the string representation of the role.
func (r NodeRole) String() string {
	return string(r)
}

// --- Directory and file name constants (private) ---

const (
	// stateDirName is the well-known directory name under $HOME.
	stateDirName = ".echoryn"

	// hivemindConfigFileName is the Hivemind server configuration file.
	hivemindConfigFileName = "hivemind.json"

	// golemConfigFileName is the Golem worker configuration file.
	golemConfigFileName = "golem.json"

	// defaultAgentID is used when no explicit agent ID is provided.
	defaultAgentID = "main"
)

// --- Environment variable keys ---

const (
	envHome     = "ECHORYN_HOME"      // Override home directory
	envStateDir = "ECHORYN_STATE_DIR" // Override state directory root
	envConfig   = "ECHORYN_CONFIG"    // Override config file path (any role)
)

// --- Project-level data directory override ---
//
// When a project root is set (via SetDataDir), ResolveStateDir returns
// <dataDir>/.echoryn instead of ~/.echoryn. This allows per-project
// isolation of all Echoryn state (sessions, memory, config).

var (
	dataDirMu sync.RWMutex
	dataDir   string // absolute path to the custom data directory; empty = use default (home-based)
)

// SetDataDir sets a custom data directory for all path resolution.
// When set, ResolveStateDir returns <dir>/.echoryn instead of ~/.echoryn.
// Pass an empty string to revert to the default (home-based) behaviour.
func SetDataDir(dir string) {
	dataDirMu.Lock()
	defer dataDirMu.Unlock()

	if dir == "" {
		dataDir = ""
		return
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	dataDir = abs
}

// GetDataDir returns the current custom data directory, or empty string if not set.
func GetDataDir() string {
	dataDirMu.RLock()
	defer dataDirMu.RUnlock()
	return dataDir
}

// --- resolve* functions: pure path computation, no I/O ---

// ResolveHomeDir returns the effective home directory.
// Priority: ECHORYN_HOME env → os.UserHomeDir().
func ResolveHomeDir() string {
	if v := os.Getenv(envHome); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// ResolveStateDir returns the root state directory.
// Priority: ECHORYN_STATE_DIR env → <dataDir>/.echoryn (if set) → ~/.echoryn.
func ResolveStateDir() string {
	if v := os.Getenv(envStateDir); v != "" {
		return v
	}

	dataDirMu.RLock()
	d := dataDir
	dataDirMu.RUnlock()

	if d != "" {
		return filepath.Join(d, stateDirName)
	}
	return filepath.Join(ResolveHomeDir(), stateDirName)
}

// ResolveConfigPath returns the config file path for the given role.
// Priority: ECHORYN_CONFIG env → <stateDir>/<role>.json.
//
// When role is RoleHivemind: ~/.echoryn/hivemind.json
// When role is RoleGolem:    ~/.echoryn/golem.json
func ResolveConfigPath(role ...NodeRole) string {
	if v := os.Getenv(envConfig); v != "" {
		return v
	}
	r := RoleHivemind
	if len(role) > 0 && role[0] != "" {
		r = role[0]
	}
	return filepath.Join(ResolveStateDir(), r.ConfigFileName())
}

// ResolveAgentDir returns the agent-specific data directory.
// Layout: <stateDir>/agents/<agentID>/
func ResolveAgentDir(agentID string) string {
	if agentID == "" {
		agentID = defaultAgentID
	}
	return filepath.Join(ResolveStateDir(), "agents", agentID)
}

// ResolveSessionStorePath returns the session BoltDB file path.
// Layout: <stateDir>/agents/<agentID>/sessions/store.db
func ResolveSessionStorePath(agentID string) string {
	return filepath.Join(ResolveAgentDir(agentID), "sessions", "store.db")
}

// ResolveMemoryDBPath returns the memory SQLite index path.
// Layout: <stateDir>/memory/<agentID>.db
func ResolveMemoryDBPath(agentID string) string {
	if agentID == "" {
		agentID = defaultAgentID
	}
	return filepath.Join(ResolveStateDir(), "memory", agentID+".db")
}

// ResolveWorkspaceDir returns the agent workspace directory.
//
// If explicit is non-empty, it is used as-is (absolute or relative).
// Otherwise, the default workspace is: <stateDir>/workspace
func ResolveWorkspaceDir(agentID, explicit string) string {
	if explicit != "" {
		if filepath.IsAbs(explicit) {
			return explicit
		}
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return explicit
		}
		return abs
	}
	return filepath.Join(ResolveStateDir(), "workspace")
}

// ResolveMCPConfigPath returns the MCP config file path.
// Layout: <stateDir>/mcp.json
func ResolveMCPConfigPath() string {
	return filepath.Join(ResolveStateDir(), "mcp.json")
}

// ResolveGolemDataDir returns the golem-specific data directory.
// Layout: <stateDir>/golem/
func ResolveGolemDataDir() string {
	return filepath.Join(ResolveStateDir(), "golem")
}

// ResolveGolemWorkspace returns the golem workspace directory.
// Layout: <stateDir>/golem/workspace
func ResolveGolemWorkspace() string {
	return filepath.Join(ResolveGolemDataDir(), "workspace")
}

// ResolveGolemSkillsDir returns the golem skills directory.
// Layout: <stateDir>/golem/skills
func ResolveGolemSkillsDir() string {
	return filepath.Join(ResolveGolemDataDir(), "skills")
}

// ResolveHivemindSkillsDir returns the hivemind-level skills directory.
// These are global decision-making skills that describe what the system
// can accomplish by orchestrating Golem nodes.
// Layout: <stateDir>/skills
func ResolveHivemindSkillsDir() string {
	return filepath.Join(ResolveStateDir(), "skills")
}

// ResolveCredentialsDir returns the credentials directory.
// Layout: <stateDir>/credentials
func ResolveCredentialsDir() string {
	return filepath.Join(ResolveStateDir(), "credentials")
}

// ResolveGolemLogsDir returns the golem skills directory.
// Layout: <stateDir>/golem/data/logs
func ResolveGolemLogsDir() string {
	return filepath.Join(ResolveGolemDataDir(), "data", "logs")
}

// ResolveTemplatesDirs returns the list of directories to search for team template.
// Priority follows the same pattern as config loading:
// 1. ./conf/templates (project-specific templates, checked into source control)
// 2. <stateDir>/templates (~/.echoryn/templates by default, user-level)
//
// The TemplateLoader will scan all directories in order, loading templates
// from each. Later directories can override templates from earlier ones.
func ResolveTemplatesDirs() []string {
	return []string{
		filepath.Join(".", "conf", "templates"),
		filepath.Join(ResolveStateDir(), "templates"),
	}
}

// ResolveAdminTokenPath returns the admin token file path.
// Layout: <stateDir>/credentials/admin_token
func ResolveAdminTokenPath() string {
	return filepath.Join(ResolveCredentialsDir(), "admin_token")
}

// DefaultAgentID returns the default agent identifier.
func DefaultAgentID() string {
	return defaultAgentID
}

// ParseRole converts a string to a NodeRole, returning RoleHivemind as default.
func ParseRole(s string) NodeRole {
	switch NodeRole(s) {
	case RoleGolem:
		return RoleGolem
	case RoleHivemind:
		return RoleHivemind
	default:
		return RoleHivemind
	}
}
