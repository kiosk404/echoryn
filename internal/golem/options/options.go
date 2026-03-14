package options

import (
	"fmt"

	"github.com/kiosk404/echoryn/internal/pkg/server"
	"github.com/kiosk404/echoryn/pkg/utils/cliflag"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// Options runs a Golem worker node.
// In the stream-based architecture, Golem does NOT listen on any port.
// It only connects to Hivemind as a client.
type Options struct {
	// HivemindAddress is the gRPC address of the Hivemind control plane.
	HivemindAddress string `json:"hivemind-address" mapstructure:"hivemind-address"`

	// JoinToken is the Bootstrap Token for registering with Hivemind.
	// Required unless Hivemind has golem.dev-mode enabled and Golem connects from loopback
	JoinToken string `json:"join-token" mapstructure:"join-token"`

	// Node configuration.
	NodeName           string            `json:"node-name"           mapstructure:"node-name"`
	NodeLabels         map[string]string `json:"node-labels"         mapstructure:"node-labels"`
	MaxConcurrentTasks int               `json:"max-concurrent-tasks" mapstructure:"max-concurrent-tasks"`

	// WorkspaceDir is the Golem workspace root directory.
	WorkspaceDir string `json:"workspace-dir" mapstructure:"workspace-dir"`

	// SkillsDir is the external skills directory (default: ~/.echoryn/golem/skills/).
	SkillsDir string `json:"skills-dir" mapstructure:"skills-dir"`

	// HotReload enables fsnotify-based hot reload for external skills.
	HotReload bool `json:"hot-reload" mapstructure:"hot-reload"`

	// HeartbeatInterval is the interval between heartbeat messages.
	HeartbeatInterval string `json:"heartbeat-interval" mapstructure:"heartbeat-interval"`

	// ConnectTimeout is the gRPC dial timeout for connecting to Hivemind.
	ConnectTimeout string `json:"connect-timeout" mapstructure:"connect-timeout"`

	// ReconnectInterval is the interval between reconnection attempts.
	ReconnectInterval string `json:"reconnect-interval" mapstructure:"reconnect-interval"`

	// DataDir specifies a custom data directory for all Echoryn state.
	DataDir string `json:"data-dir" mapstructure:"data-dir"`

	// Log rotation settings.
	LogMaxSize    int64 `json:"log-max-size" mapstructure:"log-max-size"`
	LogMaxBackups int   `json:"log-max-backups" mapstructure:"log-max-backups"`
	LogMaxAge     int   `json:"log-max-age" mapsturcture:"json:"log-max-age"`
}

// NewOptions creates default Options for a Golem worker.
func NewOptions() *Options {
	return &Options{
		HivemindAddress:    "127.0.0.1:11788",
		NodeName:           "",
		NodeLabels:         map[string]string{"env": "local"},
		MaxConcurrentTasks: 5,
		WorkspaceDir:       "",
		SkillsDir:          "",
		HotReload:          true,
		HeartbeatInterval:  "15s",
		ConnectTimeout:     "10s",
		ReconnectInterval:  "5s",
		DataDir:            "",
		LogMaxSize:         10,
		LogMaxBackups:      3,
		LogMaxAge:          7,
	}
}

// Flags returns flag sets for CLI binding.
func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	fs := fss.FlagSet("hivemind")
	fs.StringVar(&o.HivemindAddress, "hivemind-address", o.HivemindAddress,
		"gRPC address of the Hivemind control plane.")
	fs.StringVar(&o.ConnectTimeout, "connect-timeout", o.ConnectTimeout,
		"Timeout for initial gRPC connection to Hivemind.")
	fs.StringVar(&o.ReconnectInterval, "reconnect-interval", o.ReconnectInterval,
		"Interval between reconnection attempts to Hivemind.")
	fs.StringVar(&o.JoinToken, "join-token", o.JoinToken,
		"Bootstrap Token for registering with Hivemind, Required unless hivemind has golem.dev-mode enabled.")
	fs.StringVar(&o.HeartbeatInterval, "heartbeat-interval", o.HeartbeatInterval,
		"Interval between heartbeat messages sent to Hivemind.")

	ns := fss.FlagSet("node")
	ns.StringVar(&o.NodeName, "node-name", o.NodeName,
		"Node name (defaults to hostname if empty).")
	ns.IntVar(&o.MaxConcurrentTasks, "max-concurrent-tasks", o.MaxConcurrentTasks,
		"Maximum number of concurrent tasks this Golem can execute.")
	ns.StringVar(&o.WorkspaceDir, "workspace-dir", o.WorkspaceDir,
		"Workspace root directory for task execution.")

	ss := fss.FlagSet("skills")
	ss.StringVar(&o.SkillsDir, "skills-dir", o.SkillsDir,
		"External skills directory (default: ~/.echoryn/golem/skills/).")
	ss.BoolVar(&o.HotReload, "hot-reload", o.HotReload,
		"Enable fsnotify-based hot reload for external skills.")

	gs := fss.FlagSet("global")
	gs.StringVar(&o.DataDir, "data-dir", o.DataDir,
		"Custom data directory for Echoryn state. "+
			"When set, the state directory becomes <data-dir>/.echoryn instead of ~/.echoryn.")

	ls := fss.FlagSet("log")
	ls.Int64Var(&o.LogMaxSize, "log-max-size", o.LogMaxSize,
		"Max size in MB before log rotation (default: 10)")
	ls.IntVar(&o.LogMaxBackups, "log-max-backups", o.LogMaxBackups,
		"Max number of old log files to keep (default: 3)")
	ls.IntVar(&o.LogMaxAge, "log-max-age", o.LogMaxAge,
		"Max days to keep old log files (default: 7)")
	return fss
}

// Validate checks Options for invalid values.
func (o *Options) Validate() []error {
	var errs []error
	if o.HivemindAddress == "" {
		errs = append(errs, fmt.Errorf("--hivemind-address must not be empty"))
	}
	return errs
}

// Complete sets derived default values.
func (o *Options) Complete() error {
	return nil
}

// ApplyTo applies the options to the generic server config.
func (o *Options) ApplyTo(c *server.Config) error {
	return nil
}

// String returns a JSON representation.
func (o *Options) String() string {
	data, _ := json.Marshal(o)
	return string(data)
}
