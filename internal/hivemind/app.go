package hivemind

import (
	"fmt"

	"github.com/kiosk404/echoryn/internal/hivemind/config"
	"github.com/kiosk404/echoryn/internal/hivemind/options"
	"github.com/kiosk404/echoryn/pkg/app"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/paths"
)

const (
	// recommendedLogDir 定义日志输出的地址
	recommendedLogDir = "./output/"
)

const commandDesc = `The Echoryn Hivemind server`

// NewApp creates an App object with default parameters.
func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp("Echoryn Hivemind Server",
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
		// If a custom data directory is specified, configure paths package
		// so that all state goes under <data-dir>/.echoryn instead of ~/.echoryn

		if opts.DataDir != "" {
			paths.SetDataDir(opts.DataDir)
			logger.Info("[Hivemind] using custom data directory: %s (state dir: %s)",
				opts.DataDir, paths.ResolveStateDir())
		}

		logBasePath := recommendedLogDir
		logPath := fmt.Sprintf("%s%s", logBasePath, "log/common.log")

		if err := logger.InitLog(logPath); err != nil {
			panic(err)
		}
		defer logger.FlushLog()

		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}

		return Run(cfg)
	}
}
