// Package version exposes the build version injected by release builds.
package version

// Value is replaced with the Git tag through -ldflags -X in release builds.
var Value = "dev"
