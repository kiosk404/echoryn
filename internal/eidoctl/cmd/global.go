package cmd

import (
	"github.com/spf13/pflag"
)

var (
	globalEidolonHiveMindAddr string
)

func addGlobalFlags(flags *pflag.FlagSet) {
	flags.StringVar(&globalEidolonHiveMindAddr,
		"hivemind-addr",
		"127.0.0.1:11788",
		"Address of the hivemind central server (host:port)")
}

func GetHIveMindAddr() string {
	return globalEidolonHiveMindAddr
}
