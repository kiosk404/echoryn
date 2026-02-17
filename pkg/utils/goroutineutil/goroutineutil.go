package goroutineutil

import (
	"context"
	"runtime"

	"github.com/kiosk404/echoryn/pkg/logger"
)

func GoWithDeferFunc(ctx context.Context, f func()) {
	go func() {
		defer func() {
			if e := recover(); e != nil {
				const size = 64 << 10
				buf := make([]byte, size)
				buf = buf[:runtime.Stack(buf, false)]
				logger.Error("goroutine panic: %s: %s", e, buf)
			}
		}()
		f()
	}()
}

func GoWithDefaultRecovery(ctx context.Context, f func()) {
	GoWithDeferFunc(ctx, f)
}
