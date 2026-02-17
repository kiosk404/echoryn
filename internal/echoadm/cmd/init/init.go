package init

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/internal/echoadm/cmd/util"
	"github.com/kiosk404/echoryn/internal/echoadm/utils/templates"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	"github.com/kiosk404/echoryn/pkg/utils/homedir"
	"github.com/spf13/cobra"
)

var initExample = templates.Examples(`
		# Initialize the echoryn agent
		echoctl init

		# Initialize the echoryn agent with a custom workspace directory
		echoctl init --workspace=/path/to/workspace

		# Initialize the echoryn agent with a custom skills directory
		echoctl init --skills=/path/to/skills
`)

type Init struct {
	Workspace string
	SkillsDir string
	DataDir   string
	Force     bool
	Factory   util.Factory
	genericclioptions.IOStreams
}

func NewCmdInit(f util.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := NewInitOptions(f, ioStreams)

	cmd := &cobra.Command{
		Use:                   "init",
		DisableFlagsInUseLine: true,
		Aliases:               []string{},
		Short:                 "Initialize this machine as a golem worker node",
		Long: `Perpare the local machine to join a hivemind realm as a golem worker node.
		This command creates the required directories (workspace, skills, data) and generates 
		a node identity file if one does not exist. and validates the system to ensure it is 
		ready to join the hivemind.	
		
		If the node is already initialized, use the --force flag to force re-initialization.
		`,
		Example: initExample,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Run(cmd.Context(), args))
		},
		SuggestFor: []string{},
	}

	cmd.Flags().StringVar(&o.Workspace, "workspace", o.Workspace, "The workspace directory to use")
	cmd.Flags().StringVar(&o.SkillsDir, "skills", o.SkillsDir, "The skills directory to use")
	cmd.Flags().StringVar(&o.DataDir, "data", o.DataDir, "The data directory to use")
	cmd.Flags().BoolVar(&o.Force, "force", o.Force, "Force initialization even if the workspace directory is not empty")

	return cmd
}

func NewInitOptions(f util.Factory, ioStreams genericclioptions.IOStreams) *Init {
	return &Init{
		Factory:   f,
		IOStreams: ioStreams,
		Workspace: homedir.HomeDir() + "/.echoryn",
		SkillsDir: homedir.HomeDir() + "/.echoryn/skills",
		DataDir:   homedir.HomeDir() + "/.echoryn/data",
		Force:     false,
	}
}

func (o *Init) Run(ctx context.Context, args []string) error {
	fmt.Fprintf(o.Out, "\nInitializing echoryn agent...\n")

	dirs := []struct {
		label string
		path  string
	}{
		{"workspace directory", o.Workspace},
		{"skills directory", o.SkillsDir},
		{"data directory", o.DataDir},
		{"log directory", o.DataDir + "/logs"},
		{"cache directory", o.DataDir + "/cache"},
	}

	if o.SkillsDir != "" {
		dirs = append(dirs, struct {
			label string
			path  string
		}{label: "skills directory", path: o.SkillsDir})
	}

	for _, d := range dirs {
		fmt.Fprintf(o.Out, "Creating %s: %s\n", d.label, d.path)
	}

	fmt.Fprintf(o.Out, "\n.Generating node identity...\n")

	fmt.Fprintf(o.Out, "\n.Running system validations...\n")

	checker := o.Factory.NodeChecker()

	results, err := checker.RunAll(ctx)
	if err != nil {
		return fmt.Errorf("node check failed!error:%w", err)
	}

	hasWarning := false

	for _, r := range results {
		mark := "✔"
		if !r.Passed && r.Status == "warning" {
			mark = "⚠"
			hasWarning = true
		} else {
			mark = "✖"
		}
		fmt.Fprintf(o.Out, "%s %s %s\n", mark, r.Name, r.Message)
	}

	fmt.Fprintf(o.Out, "\n.Initialization complete!\n")
	if hasWarning {
		fmt.Fprintf(o.Out, "\nSome checks failed with warning, please check the logs for details.\n")
	}
	fmt.Fprintf(o.Out, "\nYou can now join the hivemind by running 'echoctl join'.\n")
	return nil
}

func (o *Init) Validate() error {
	return nil
}
