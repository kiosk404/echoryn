package app

import (
	"github.com/kiosk404/echoryn/pkg/utils/cliflag"
)

// CliOptions abstracts configuration options for reading parameters from the
// command line.
type CliOptions interface {
	// Flags AddFlags adds flags to the specified FlagSet object.
	// AddFlags(fs *pflag.FlagSet)
	Flags() (fss cliflag.NamedFlagSets)
	Validate() []error
}

// ConfigurableOptions abstracts configuration options for reading parameters
// from a configuration file.
type ConfigurableOptions interface {
	// ApplyFlags parsing parameters from the command line or configuration file
	// to the options instance.
	ApplyFlags() []error
}

// CompletableOptions abstracts options which can be completed.
type CompletableOptions interface {
	Complete() error
}

// PrintableOptions abstracts options which can be printed.
type PrintableOptions interface {
	String() string
}
