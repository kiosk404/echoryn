package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/kiosk404/eidolon/internal/eidoctl"
)

func main() {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	command := eidoctl.NewDefaultHivCtlCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
