// Package devtools defines optional local developer-tool modes.
package devtools

import "strings"

// Config is resolved once at process startup and then passed to consumers.
// Release builds always receive the zero value.
type Config struct {
	Enabled            bool
	ForceLog           bool
	IdleMonitorSeconds int
	InputTrace         bool
	CapturePanel       bool
	WarningPreview     bool
	Notices            []string
}

func (c Config) IdleMonitorEnabled() bool { return c.Enabled && c.IdleMonitorSeconds > 0 }

func (c Config) Summary() string {
	parts := make([]string, 0, 4)
	if c.IdleMonitorEnabled() {
		parts = append(parts, "idle_monitor")
	}
	if c.InputTrace {
		parts = append(parts, "input_trace")
	}
	if c.CapturePanel {
		parts = append(parts, "capture_panel")
	}
	if c.WarningPreview {
		parts = append(parts, "warning_preview")
	}
	return strings.Join(parts, ",")
}
