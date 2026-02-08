package options

import (
	genericoptions "github.com/kiosk404/eidolon/internal/pkg/options"
	"github.com/kiosk404/eidolon/internal/pkg/server"
	"github.com/kiosk404/eidolon/pkg/utils/cliflag"
	"github.com/kiosk404/eidolon/pkg/utils/json"
)

type Options struct {
	GRPCOptions             *genericoptions.GRPCOptions      `json:"grpc"     mapstructure:"grpc"`
	GenericServerRunOptions *genericoptions.ServerRunOptions `json:"serving"     mapstructure:"serving"`
}

func (o *Options) Flags() (fss cliflag.NamedFlagSets) {
	o.GRPCOptions.AddFlags(fss.FlagSet("grpc"))
	o.GenericServerRunOptions.AddFlags(fss.FlagSet("generic"))

	return fss
}

func NewOptions() *Options {
	return &Options{
		GRPCOptions:             genericoptions.NewGRPCOptions(),
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
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
