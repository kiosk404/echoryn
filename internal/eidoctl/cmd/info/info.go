package info

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	cmdutil "github.com/kiosk404/eidolon/internal/eidoctl/cmd/util"
	"github.com/kiosk404/eidolon/pkg/cli/genericclioptions"
	"github.com/kiosk404/eidolon/pkg/utils/iputil"
	"github.com/kiosk404/eidolon/pkg/utils/templates"
	hoststat "github.com/likexian/host-stat-go"
	"github.com/spf13/cobra"
)

var infoExample = templates.Examples(`
		# Print the host information
		eidoctl info`)

// Info is an options struct to support 'info' sub command.
type Info struct {
	HostName  string
	IPAddress string
	OSRelease string
	CPUCore   uint64
	MemTotal  string
	MemFree   string
	genericclioptions.IOStreams
}

// NewInfoOptions returns an initialized InfoOptions instance.
func NewInfoOptions(ioStreams genericclioptions.IOStreams) *Info {
	return &Info{
		IOStreams: ioStreams,
	}
}

// NewCmdInfo returns new initialized instance of 'info' sub command.
func NewCmdInfo(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := NewInfoOptions(ioStreams)

	cmd := &cobra.Command{
		Use:                   "info",
		DisableFlagsInUseLine: true,
		Aliases:               []string{},
		Short:                 "Print the host information",
		Long:                  "Print the host information.",
		Example:               infoExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Run(cmd.Context(), args))
		},
		SuggestFor: []string{},
	}

	return cmd
}

// Run executes an info sub command using the specified options.
func (o *Info) Run(ctx context.Context, args []string) error {
	var info Info

	hostInfo, err := hoststat.GetHostInfo()
	if err != nil {
		return fmt.Errorf("get host info failed!error:%w", err)
	}

	info.HostName = hostInfo.HostName
	info.OSRelease = hostInfo.Release + " " + hostInfo.OSBit

	memStat, err := hoststat.GetMemStat()
	if err != nil {
		return fmt.Errorf("get mem stat failed!error:%w", err)
	}

	info.MemTotal = strconv.FormatUint(memStat.MemTotal, 10) + "M"
	info.MemFree = strconv.FormatUint(memStat.MemFree, 10) + "M"
	info.IPAddress = iputil.GetLocalIP()

	cpuStat, err := hoststat.GetCPUInfo()
	if err != nil {
		return fmt.Errorf("get cpu stat failed!error:%w", err)
	}

	info.CPUCore = cpuStat.CoreCount

	s := reflect.ValueOf(&info).Elem()
	typeOfInfo := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)

		v := fmt.Sprintf("%v", f.Interface())
		if v != "" {
			fmt.Fprintf(o.Out, "%12s %v\n", typeOfInfo.Field(i).Name+":", f.Interface())
		}
	}

	return nil
}
