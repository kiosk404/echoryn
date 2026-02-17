package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/kiosk404/echoryn/internal/echoctl/cmd"
)

func main() {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	command := cmd.NewDefaultEchoCtlCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
