//go:build !windows

package inputdiag

// Start is a no-op on non-Windows platforms.
func Start(bool) func() { return nil }
