package cmd

import (
	"github.com/spf13/pflag"
)

var (
	globalEchorynHiveMindAddr string
)

func addGlobalFlags(flags *pflag.FlagSet) {
	flags.StringVar(&globalEchorynHiveMindAddr,
		"hivemind-addr",
		"127.0.0.1:11788",
		"Address of the hivemind central server (host:port)")
}

// HiveMindAddrPtr returns a pointer to the global Hivemind address variable.
// Used to pass to Factory so subcommands can access it without importing this package.
func HiveMindAddrPtr() *string {
	return &globalEchorynHiveMindAddr
}
