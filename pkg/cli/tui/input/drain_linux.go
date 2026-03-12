//go:build linux

package input

import (
	"os"

	"golang.org/x/sys/unix"
)

// drainStdout blocks until all data written to stdout has been
// transmitted to the terminal device.
//
// On Linux, ioctl(fd, TCSBRK, 1) is equivalent to tcdrain(fd).
// See tty_ioctl(4).
//
// If the call fails (e.g. stdout is redirected to a pipe) it is
// silently ignored — this is a best-effort optimisation.
func drainStdout() {
	_ = unix.IoctlSetInt(int(os.Stdout.Fd()), unix.TCSBRK, 1)
}
