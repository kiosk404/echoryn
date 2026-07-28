// Package localfileops implements the Hivemind-side local file-operation
// plugin, which exposes read_file / write_file / patch / search_files tools
// backed by pkg/fileops against the Hivemind machine's local filesystem.
//
// Default configuration: read enabled (no root restriction), write disabled.
// Operators must explicitly enable write AND provide allowed_roots for the
// Agent to modify files on the Hivemind machine; otherwise write_file and
// patch are simply not registered, and the LLM doesn't see them.
package localfileops

// Config is the runtime configuration for the local-fileops plugin.
type Config struct {
	// Enabled toggles the whole plugin. When false, no tools are registered.
	Enabled bool `json:"enabled"`

	// Read controls the read_file and search_files tools.
	Read ReadConfig `json:"read"`

	// Write controls the write_file and patch tools. Disabled by default
	// for safety -- the Hivemind node is a control plane, not an
	// execution target.
	Write WriteConfig `json:"write"`

	// DenyExact / DenyPrefix extend the builtin deny list (always applied
	// in addition to the hardcoded credentials/system paths in pkg/fileops).
	DenyExact  []string `json:"deny_exact"`
	DenyPrefix []string `json:"deny_prefix"`
}

// ReadConfig controls the read_file and search_files tools.
type ReadConfig struct {
	Enabled      bool     `json:"enabled"`
	AllowedRoots []string `json:"allowed_roots"` // empty = no root restriction (still deny-guarded)
	MaxBytes     int64    `json:"max_bytes"`
}

// WriteConfig controls the write_file and patch tools.
type WriteConfig struct {
	Enabled      bool     `json:"enabled"`
	AllowedRoots []string `json:"allowed_roots"` // required when Enabled=true
}

// DefaultConfig returns sensible defaults: plugin enabled, read on
// (unrestricted within builtin deny), write off. Operators must explicitly
// opt-in to write.
func DefaultConfig() *Config {
	return &Config{
		Enabled: true,
		Read: ReadConfig{
			Enabled:  true,
			MaxBytes: 5 * 1024 * 1024,
		},
		Write: WriteConfig{
			Enabled: false,
		},
	}
}
