package paths

import (
	"fmt"
	"os"
)

// EnsureStateDir creates the Hivemind state directory structure if it does not exist.
// Called once at startup for the Hivemind control plane. Returns the resolved stateDir path.
//
// Creates:
//
//	~/.echoryn/
//	├── agents/
//	│   └── main/
//	│       └── sessions/
//	├── memory/
//	├── workspace/
//	│   ├── memory/
//	│   └── prompts/
//	└── credentials/
func EnsureStateDir() (string, error) {
	return EnsureStateDirForRole(RoleHivemind)
}

// EnsureStateDirForRole creates the state directory structure for the given node role.
// Hivemind and Golem have different directory layouts.
func EnsureStateDirForRole(role NodeRole) (string, error) {
	stateDir := ResolveStateDir()

	// Common base directory.
	dirs := []string{stateDir}

	switch role {
	case RoleGolem:
		// Golem directory structure:
		//   ~/.echoryn/
		//   └── golem/
		//       ├── workspace/
		//       ├── skills/
		//       └── data/
		//           ├── logs/
		//           └── cache/
		golemDir := ResolveGolemDataDir()
		dirs = append(dirs,
			golemDir,
			ResolveGolemWorkspace(),
			ResolveGolemSkillsDir(),
			resolveSubDir(golemDir, "data"),
			resolveSubDir(golemDir, "data/logs"),
			resolveSubDir(golemDir, "data/cache"),
		)
	default:
		// Hivemind directory structure:
		//   ~/.echoryn/
		//   ├── agents/main/sessions/
		//   ├── memory/
		//   ├── workspace/{memory,prompts}/
		//   └── credentials/
		dirs = append(dirs,
			ResolveAgentDir(defaultAgentID),
			resolveSubDir(ResolveAgentDir(defaultAgentID), "sessions"),
			resolveSubDir(stateDir, "memory"),
			ResolveWorkspaceDir(defaultAgentID, ""),
			resolveSubDir(ResolveWorkspaceDir(defaultAgentID, ""), "memory"),
			resolveSubDir(ResolveWorkspaceDir(defaultAgentID, ""), "prompts"),
			resolveSubDir(stateDir, "credentials"),
		)
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return stateDir, nil
}

// EnsureAgentDirs creates agent-specific subdirectories for the given agentID.
func EnsureAgentDirs(agentID string) error {
	dirs := []string{
		ResolveAgentDir(agentID),
		resolveSubDir(ResolveAgentDir(agentID), "sessions"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return nil
}

// EnsureWorkspaceDir ensures the workspace directory and its convention
// subdirectories exist.
func EnsureWorkspaceDir(wsDir string) error {
	dirs := []string{
		wsDir,
		resolveSubDir(wsDir, "memory"),
		resolveSubDir(wsDir, "prompts"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return nil
}

// resolveSubDir is a helper that joins a parent and child directory path.
func resolveSubDir(parent, child string) string {
	return parent + "/" + child
}
