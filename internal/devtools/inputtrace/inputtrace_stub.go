//go:build !windows || !devtools

package inputtrace

// Start is unavailable outside Windows devtools builds.
func Start(bool) func() { return nil }
