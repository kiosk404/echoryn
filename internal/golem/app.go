package golem

import (
	"fmt"

	"github.com/kiosk404/eidolon/internal/hivemind/options"
	"github.com/kiosk404/eidolon/pkg/app"
	"github.com/kiosk404/eidolon/pkg/logger"
)

const (
	AppName = "golem"
)

func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp("golem",
		basename,
		app.WithOptions(opts),
		app.WithDescription(`The golem is a worker node in the eidolon realm.`),
		app.WithDefaultValidArgs(),
		app.WithRunFunc(run(opts)),
	)
	return application
}

func run(opts *options.Options) app.RunFunc {
	return func(basename string) error {
		logBaseName := basename
		logPath := fmt.Sprintf("%s/%s.log", logBaseName, logBaseName)

		if err := logger.InitLog(logPath); err != nil {
			return err
		}
		defer logger.FlushLog()

		return nil
	}
}
