package cmd

import (
	"github.com/kiosk404/echoryn/internal/echoadm/globals"
	"github.com/spf13/pflag"
)

func addGlobalFlags(flags *pflag.FlagSet) {
	flags.StringVar(&globals.ConfigPath,
		"config",
		"",
		"Path to the echoryn config file (default: ~/.echoryn/config.json)")
}
