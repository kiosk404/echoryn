package info

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	cmdutil "github.com/kiosk404/echoryn/internal/echoadm/cmd/util"
	admconfig "github.com/kiosk404/echoryn/internal/echoadm/config"
	"github.com/kiosk404/echoryn/internal/echoadm/globals"
	"github.com/kiosk404/echoryn/internal/echoadm/utils/templates"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	"github.com/kiosk404/echoryn/pkg/utils/iputil"
	"github.com/kiosk404/echoryn/pkg/utils/json"
	"github.com/kiosk404/echoryn/pkg/version"
	hoststat "github.com/likexian/host-stat-go"
	"github.com/spf13/cobra"
)

var infoExample = templates.Examples(`
		# Print node and host information
		echoadm info

		# Print in JSON format
		echoadm info --output json
`)

// Info holds the collected system information.
type Info struct {
	// Node info (from config).
	NodeRole string `json:"node_role,omitempty"`
	NodeName string `json:"node_name,omitempty"`
	NodeID   string `json:"node_id,omitempty"`

	// Host info.
	HostName  string `json:"hostname"`
	IPAddress string `json:"ip_address"`
	OSRelease string `json:"os_release"`
	CPUCore   uint64 `json:"cpu_core"`
	MemTotal  string `json:"mem_total"`
	MemFree   string `json:"mem_free"`

	// Version.
	Version string `json:"version"`
}

// InfoOptions holds the command options.
type InfoOptions struct {
	Output string
	genericclioptions.IOStreams
}

func NewCmdInfo(_ cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := &InfoOptions{
		Output:    "text",
		IOStreams: ioStreams,
	}

	cmd := &cobra.Command{
		Use:     "info",
		Short:   "Print node and host diagnostic information",
		Long:    "Print detailed information about the current node, including host hardware, OS, network, and echoryn configuration.",
		Example: infoExample,
		Run: func(cmd *cobra.Command, args []string) {
			if err := o.Run(cmd.Context()); err != nil {
				fmt.Fprintf(o.ErrOut, "error: %v\n", err)
			}
		},
	}

	cmd.Flags().StringVarP(&o.Output, "output", "o", o.Output, "Output format: text or json")

	return cmd
}

func (o *InfoOptions) Run(ctx context.Context) error {
	var info Info

	// Collect node info from config.
	configPath := globals.ConfigPath
	cfg, err := admconfig.Load(configPath)
	if err == nil && cfg.Node.Role != "" {
		info.NodeRole = cfg.Node.Role
		info.NodeName = cfg.Node.Name
		info.NodeID = cfg.Node.ID
	}

	// Host info.
	hostInfo, err := hoststat.GetHostInfo()
	if err != nil {
		return fmt.Errorf("get host info: %w", err)
	}
	info.HostName = hostInfo.HostName
	info.OSRelease = hostInfo.Release + " " + hostInfo.OSBit

	memStat, err := hoststat.GetMemStat()
	if err != nil {
		return fmt.Errorf("get mem stat: %w", err)
	}
	info.MemTotal = strconv.FormatUint(memStat.MemTotal, 10) + "M"
	info.MemFree = strconv.FormatUint(memStat.MemFree, 10) + "M"
	info.IPAddress = iputil.GetLocalIP()

	cpuStat, err := hoststat.GetCPUInfo()
	if err != nil {
		return fmt.Errorf("get cpu stat: %w", err)
	}
	info.CPUCore = cpuStat.CoreCount
	info.Version = version.Get().String()

	// Output.
	if o.Output == "json" {
		data, _ := json.MarshalIndent(info, "", "  ")
		fmt.Fprintln(o.Out, string(data))
		return nil
	}

	// Text output.
	s := reflect.ValueOf(&info).Elem()
	typeOfInfo := s.Type()
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		v := fmt.Sprintf("%v", f.Interface())
		if v != "" && v != "0" {
			fmt.Fprintf(o.Out, "%12s %v\n", typeOfInfo.Field(i).Name+":", f.Interface())
		}
	}

	return nil
}
