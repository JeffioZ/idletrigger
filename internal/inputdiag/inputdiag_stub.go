//go:build !windows || !devtools

package inputdiag

// Start is unavailable outside Windows devtools builds.
func Start(bool) func() { return nil }
