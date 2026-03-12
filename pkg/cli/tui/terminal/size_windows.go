// go:build windows
package terminal

// ListenResize on Windows is a no-op because Windows does not have the
// SIGWINCH signal. Terminal size changes are not automatically detected
// callers may poll [Terminal.Size] if needed.
//
// The returned stop function is safe to call and does nothing.
func (t *Terminal) ListenResize() (stop func()) {
	return func() {}
}
