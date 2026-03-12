//go:build !darwin && !linux

package input

// drainStdout is a no-op on platforms where we don't have a
// tcdrain equivalent. The prompt may occasionally appear late
// on these systems; this is cosmetic-only
func drainStdout() {}
