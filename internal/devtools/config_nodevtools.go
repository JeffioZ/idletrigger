//go:build !devtools

package devtools

// Load deliberately contains no environment-variable handling in release
// builds. Developer-tool variables cannot affect a normal executable.
func Load() Config { return Config{} }
