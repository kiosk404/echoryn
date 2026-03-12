//go:build !windows

package terminal

import (
	"os"
	"os/signal"
	"syscall"
)

// ListenResize starts a goroutine that listens for SIGWINCH and
// refreshes the cached terminal size. Call the returned stop function
// to clean up the signal channel.
//
// Typical usage:
//
//	stop := t.ListenResize()
//	defer stop()
func (t *Terminal) ListenResize() (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				t.refreshSize()
			case <-done:
				return
			}
		}
	}()

	return func() {
		signal.Stop(ch)
		close(done)
		close(ch)
	}
}
