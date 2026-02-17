package options

import (
	"fmt"

	"github.com/kiosk404/echoryn/internal/pkg/server"
	"github.com/spf13/pflag"
)

// ServerRunOptions contains the options while running a generic api server.
type ServerRunOptions struct {
	Mode        string   `json:"mode"        mapstructure:"mode"`
	Healthz     bool     `json:"healthz"     mapstructure:"healthz"`
	Middlewares []string `json:"middlewares" mapstructure:"middlewares"`
	BindAddress string   `json:"bind-address" mapstructure:"bind-address"`
	BindPort    int      `json:"bind-port"    mapstructure:"bind-port"`
}

// NewServerRunOptions creates a new ServerRunOptions object with default parameters.
func NewServerRunOptions() *ServerRunOptions {
	defaults := server.NewConfig()

	return &ServerRunOptions{
		Mode:        defaults.Mode,
		Healthz:     defaults.Healthz,
		Middlewares: defaults.Middlewares,
		BindAddress: defaults.Serving.BindAddress,
		BindPort:    defaults.Serving.BindPort,
	}
}

// ApplyTo applies the run options to the method receiver and returns self.
func (s *ServerRunOptions) ApplyTo(c *server.Config) error {
	c.Mode = s.Mode
	c.Healthz = s.Healthz
	c.Middlewares = s.Middlewares
	c.Serving.BindAddress = s.BindAddress
	c.Serving.BindPort = s.BindPort

	return nil
}

// Validate checks validation of ServerRunOptions.
func (s *ServerRunOptions) Validate() []error {
	errors := []error{}

	if s.BindPort < 1 || s.BindPort > 65535 {
		errors = append(
			errors,
			fmt.Errorf(
				"--server.bind-port %v must be between 1 and 65535, inclusive. It cannot be turned off with 0",
				s.BindPort,
			),
		)
	} else if s.BindPort < 0 || s.BindPort > 65535 {
		errors = append(errors, fmt.Errorf("--server.bind-port %v must be between 0 and 65535, inclusive. 0 for turning off server port", s.BindPort))
	}

	return errors
}

// AddFlags adds flags for a specific APIServer to the specified FlagSet.
func (s *ServerRunOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.BindAddress, "server.bind-address", s.BindAddress,
		"The IP address on which to listen for the --server.bind-port port. The "+
			"associated interface(s) must be reachable by the rest of the engine, and by CLI/web "+
			"clients. If blank, all interfaces will be used (0.0.0.0 for all IPv4 interfaces and :: for all IPv6 interfaces).")
	fs.IntVar(&s.BindPort, "server.bind-port", s.BindPort,
		"Bind port for the server.")
	// Note: the weird ""+ in below lines seems to be the only way to get gofmt to
	// arrange these text blocks sensibly. Grrr.
	fs.StringVar(&s.Mode, "server.mode", s.Mode, ""+
		"Start the server in a specified server mode. Supported server mode: debug, test, release.")

	fs.BoolVar(&s.Healthz, "server.healthz", s.Healthz, ""+
		"Add self readiness check and install /healthz router.")

	fs.StringSliceVar(&s.Middlewares, "server.middlewares", s.Middlewares, ""+
		"List of allowed middlewares for server, comma separated. If this list is empty default middlewares will be used.")
}
