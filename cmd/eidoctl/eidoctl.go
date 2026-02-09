package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/kiosk404/eidolon/internal/eidoctl/cmd"
)

func main() {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	command := cmd.NewDefaultHivCtlCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
