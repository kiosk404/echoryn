package golem

import (
	"fmt"

	"github.com/kiosk404/echoryn/internal/golem/config"
	"github.com/kiosk404/echoryn/internal/golem/options"
	"github.com/kiosk404/echoryn/pkg/app"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/paths"
)

const (
	AppName = "golem"
)

const commandDesc = `The golem is a worker node in the echoryn realm. - executes Skills dispatched by Hivemind.`

func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp("echoryn golem",
		basename,
		app.WithOptions(opts),
		app.WithDescription(commandDesc),
		app.WithDefaultValidArgs(),
		app.WithRunFunc(run(opts)),
	)
	return application
}

func run(opts *options.Options) app.RunFunc {
	return func(basename string) error {
		if opts.DataDir != "" {
			paths.SetDataDir(opts.DataDir)
			logger.Info("[Golem] using custom data directory: %s (state dir: %s)", opts.DataDir,
				paths.ResolveStateDir())
		}

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

		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return nil
		}

		return Run(cfg)
	}
}
