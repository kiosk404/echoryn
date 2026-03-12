//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package input

import (
	"os"

	"golang.org/x/sys/unix"
)

// drainStdout blocks until all data written to stdout has been
// transmitted to the terminal device.
//
// On macOS / BSD this uses the TIOCDRAIN ioctl, which is the
// equivalent of POSIX tcdrain(3).
//
// If the call fails (e.g. stdout is redirected to a pipe) it is
// silently ignored — this is a best-effort optimisation.
func drainStdout() {
	_ = unix.IoctlSetInt(int(os.Stdout.Fd()), unix.TIOCDRAIN, 0)
}
