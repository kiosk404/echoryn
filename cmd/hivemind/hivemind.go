package main

import (
	"math/rand"
	"time"

	"github.com/kiosk404/eidolon/internal/hivemind"
	_ "go.uber.org/automaxprocs"
)

func main() {
	rand.New(rand.NewSource(time.Now().UTC().UnixNano()))

	hivemind.NewApp("hivemind-server").Run()
}
