package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/kiosk404/echoryn/internal/echoadm/cmd"
)

func main() {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	command := cmd.NewDefaultEchoAdmCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
