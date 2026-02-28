package golem

import (
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/options"
	"github.com/kiosk404/echoryn/pkg/app"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/paths"
)

const (
	AppName = "golem"
)

func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp("golem",
		basename,
		app.WithOptions(opts),
		app.WithDescription(`The golem is a worker node in the echoryn realm.`),
		app.WithDefaultValidArgs(),
		app.WithRunFunc(run(opts)),
	)
	return application
}

func run(opts *options.Options) app.RunFunc {
	return func(basename string) error {
		// If a custom data directory is specified, configure paths package
		// so that all state goes under <data-dir>/.echoryn instead of ~/.echoryn
		if opts.DataDir != "" {
			paths.SetDataDir(opts.DataDir)
		}

		logBaseName := basename
		logPath := fmt.Sprintf("%s/%s.log", logBaseName, logBaseName)

		if err := logger.InitLog(logPath); err != nil {
			return err
		}
		defer logger.FlushLog()

		return nil
	}
}
