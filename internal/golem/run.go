package golem

import (
	"github.com/kiosk404/echoryn/internal/golem/config"
)

func Run(cfg *config.Config) error {
	server, err := createGolemServer(cfg)
	if err != nil {
		return err
	}
	return server.PrepareRun().Run()
}
