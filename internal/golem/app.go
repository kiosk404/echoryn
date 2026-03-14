package golem

import (
	"fmt"
	"path/filepath"

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

		// Ensure state directory structure (creates logs dir among others).
		stateDir, err := paths.EnsureStateDirForRole(paths.RoleGolem)
		if err != nil {
			return fmt.Errorf("failed to ensure golem state directory: %w", err)
		}
		logger.Info("[Golem] state directory: %s", stateDir)

		logPath := filepath.Join(paths.ResolveGolemLogsDir(), "golem.log")

		// Initialize logger with custom rotation config.
		rotateCfg := logger.RotateConfig{
			MaxSize:    opts.LogMaxSize,
			MaxBackups: opts.LogMaxBackups,
			MaxAge:     opts.LogMaxAge,
		}
		if err := logger.InitLogWithConfig(logPath, rotateCfg); err != nil {
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
