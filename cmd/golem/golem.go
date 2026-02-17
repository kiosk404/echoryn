package main

import (
	"math/rand"
	"time"

	"github.com/kiosk404/echoryn/internal/golem"
)

func main() {
	rand.New(rand.NewSource(time.Now().UTC().UnixNano()))

	golem.NewApp("golem-worker").Run()
}
