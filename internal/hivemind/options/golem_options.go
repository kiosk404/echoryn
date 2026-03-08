package options

// GolemOptions holds options specific to the Golem subsystem on the Hivemind side.
type GolemOptions struct {
	// DevMode enables local development mode for Golem node registration.
	// When true, nodes connecting from loopback addresses (127.0.0.0/8, ::1)
	// are allowed to register without a join token.
	// When false (default), ALL nodes must present a valid join token.
	DevMode bool `json:"dev-mode" mapstructure:"dev-mode"`
}
