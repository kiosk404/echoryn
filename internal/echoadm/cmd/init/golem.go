package init

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	cmdutil "github.com/kiosk404/echoryn/internal/echoadm/cmd/util"
	admconfig "github.com/kiosk404/echoryn/internal/echoadm/config"
	"github.com/kiosk404/echoryn/internal/echoadm/globals"
	"github.com/kiosk404/echoryn/internal/echoadm/utils/templates"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	"github.com/kiosk404/echoryn/pkg/utils/homedir"
	"github.com/spf13/cobra"
)

var golemExample = templates.Examples(`
		# Initialize a Golem worker node with defaults
		echoadm init golem

		# Initialize with a custom workspace directory
		echoadm init golem --workspace /data/echoryn/workspace

		# Force re-initialization
		echoadm init golem --force
`)

// InitGolem holds options for the 'init golem' command.
type InitGolem struct {
	Workspace string
	SkillsDir string
	DataDir   string
	NodeName  string
	Force     bool

	genericclioptions.IOStreams
}

func NewCmdInitGolem(_ cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	baseDir := filepath.Join(homedir.HomeDir(), admconfig.DefaultConfigDir, "golem")
	o := &InitGolem{
		Workspace: filepath.Join(baseDir, "workspace"),
		SkillsDir: filepath.Join(baseDir, "skills"),
		DataDir:   filepath.Join(baseDir, "data"),
		IOStreams: ioStreams,
	}

	cmd := &cobra.Command{
		Use:     "golem",
		Short:   "Initialize a Golem worker node",
		Long:    `Initialize this machine as a Golem worker node. This creates the workspace, skills, data, log, and cache directories and prepares the node for joining a Hivemind cluster.`,
		Example: golemExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Run(cmd.Context()))
		},
	}

	cmd.Flags().StringVar(&o.Workspace, "workspace", o.Workspace, "Agent workspace directory")
	cmd.Flags().StringVar(&o.SkillsDir, "skills-dir", o.SkillsDir, "Skills directory")
	cmd.Flags().StringVar(&o.DataDir, "data-dir", o.DataDir, "Data directory")
	cmd.Flags().StringVar(&o.NodeName, "node-name", o.NodeName, "Node name (defaults to hostname)")
	cmd.Flags().BoolVar(&o.Force, "force", o.Force, "Force re-initialization")

	return cmd
}

func (o *InitGolem) Run(ctx context.Context) error {
	configPath := globals.ConfigPath
	if configPath == "" {
		configPath = admconfig.DefaultConfigPath()
	}

	// Check if already initialized as golem.
	if admconfig.Exists(configPath) && !o.Force {
		cfg, err := admconfig.Load(configPath)
		if err == nil && cfg.Node.Role == "golem" {
			return fmt.Errorf("already initialized as golem at %s. Use --force to re-initialize", configPath)
		}
	}

	fmt.Fprintf(o.Out, "\n[init] Initializing Golem worker node...\n\n")

	// Create directories.
	dirs := []struct {
		label string
		path  string
	}{
		{"base directory", admconfig.DefaultBaseDir()},
		{"workspace", o.Workspace},
		{"skills", o.SkillsDir},
		{"data", o.DataDir},
		{"logs", filepath.Join(o.DataDir, "logs")},
		{"cache", filepath.Join(o.DataDir, "cache")},
	}

	for _, d := range dirs {
		fmt.Fprintf(o.Out, "[dirs] Creating %s: %s\n", d.label, d.path)
		if err := os.MkdirAll(d.path, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", d.label, err)
		}
	}
	fmt.Fprintln(o.Out)

	// Generate node identity.
	nodeName := o.NodeName
	if nodeName == "" {
		nodeName = getHostname()
	}
	nodeID := generateTokenID()

	fmt.Fprintf(o.Out, "[identity] Node name: %s\n", nodeName)
	fmt.Fprintf(o.Out, "[identity] Node ID:   %s\n\n", nodeID)

	// Save global config.
	cfg := admconfig.DefaultConfig()
	if admconfig.Exists(configPath) {
		loaded, err := admconfig.Load(configPath)
		if err == nil {
			cfg = loaded
		}
	}

	cfg.Node.Role = "golem"
	cfg.Node.Name = nodeName
	cfg.Node.ID = nodeID
	cfg.Golem.Workspace = o.Workspace
	cfg.Golem.SkillsDir = o.SkillsDir
	cfg.Golem.DataDir = o.DataDir

	if err := admconfig.Save(configPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Fprintf(o.Out, "[config] Saved global config: %s\n", configPath)

	// Print summary.
	fmt.Fprintf(o.Out, "\n%s\n", "============================================")
	fmt.Fprintf(o.Out, " Golem worker node initialized!\n")
	fmt.Fprintf(o.Out, "%s\n\n", "============================================")
	fmt.Fprintf(o.Out, "  Node Name:  %s\n", nodeName)
	fmt.Fprintf(o.Out, "  Workspace:  %s\n", o.Workspace)
	fmt.Fprintf(o.Out, "  Skills:     %s\n", o.SkillsDir)
	fmt.Fprintf(o.Out, "  Data:       %s\n", o.DataDir)
	fmt.Fprintf(o.Out, "\nTo join a Hivemind cluster:\n")
	fmt.Fprintf(o.Out, "  echoadm join --token <join-token> --hivemind-addr <addr>\n\n")

	return nil
}
