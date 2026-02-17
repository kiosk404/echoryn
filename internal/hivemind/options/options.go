package options

import (
	genericoptions "github.com/kiosk404/echoryn/internal/pkg/options"
	"github.com/kiosk404/echoryn/internal/pkg/server"
	"github.com/kiosk404/echoryn/pkg/utils/cliflag"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

type Options struct {
	GRPCOptions             *genericoptions.GRPCOptions      `json:"grpc"     mapstructure:"grpc"`
	GenericServerRunOptions *genericoptions.ServerRunOptions `json:"serving"  mapstructure:"serving"`
	ModelOptions            *genericoptions.ModelOptions     `json:"models"   mapstructure:"models"`
	PluginOptions           *genericoptions.PluginsOptions   `json:"plugins"  mapstructure:"plugins"`
	MCPOptions              *MCPOptions                      `json:"mcp"      mapstructure:"mcp"`
}

func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	o.GRPCOptions.AddFlags(fss.FlagSet("grpc"))
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("generic"))
	o.ModelOptions.AddFlags(fss.FlagSet("models"))
	o.PluginOptions.AddFlags(fss.FlagSet("plugins"))
	o.MCPOptions.AddFlags(fss.FlagSet("mcp"))
	return fss
}

func NewOptions() *Options {
	return &Options{
		GRPCOptions:             genericoptions.NewGRPCOptions(),
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		ModelOptions:            genericoptions.NewModelOptions(),
		PluginOptions:           genericoptions.NewPluginsOptions(),
		MCPOptions:              NewMCPOptions(),
	}
}

// ApplyTo applies the run options to the method receiver and returns self.
func (o *Options) ApplyTo(c *server.Config) error {
	return nil
}

func (o *Options) String() string {
	data, _ := json.Marshal(o)

	return string(data)
}

// Complete set default Options.
func (o *Options) Complete() error {
	return nil
}
