//go:build !windows

package inputdiag

// Start is a no-op on non-Windows platforms.
func Start() func() { return nil }
