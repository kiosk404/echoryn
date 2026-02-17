package hivemind

import (
	"github.com/kiosk404/echoryn/internal/hivemind/config"
)

func Run(cfg *config.Config) error {
	server, err := createAPIServer(cfg)
	if err != nil {
		return err
	}

	return server.PrepareRun().Run()
}
