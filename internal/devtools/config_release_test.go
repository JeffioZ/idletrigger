//go:build !devtools

package devtools

import "testing"

func TestReleaseBuildIgnoresDeveloperToolVariables(t *testing.T) {
	for _, name := range []string{
		"IDLETRIGGER_DEVTOOLS",
		"IDLETRIGGER_DEVTOOLS_LOG",
		"IDLETRIGGER_DEVTOOLS_IDLE_MONITOR_SECONDS",
		"IDLETRIGGER_DEVTOOLS_INPUT_TRACE",
		"IDLETRIGGER_DEVTOOLS_CAPTURE_PANEL",
		"IDLETRIGGER_DEVTOOLS_WARNING_PREVIEW",
		"IDLETRIGGER_DEBUG_LOG",
		"IDLETRIGGER_IDLE_TEST_SECONDS",
		"IDLETRIGGER_INPUT_DIAGNOSTICS",
		"IDLETRIGGER_CAPTURE_MODE",
		"IDLETRIGGER_DEV",
	} {
		t.Setenv(name, "1")
	}
	if got := Load(); got.Enabled || got.ForceLog || got.IdleMonitorSeconds != 0 || got.InputTrace || got.CapturePanel || got.WarningPreview || len(got.Notices) != 0 {
		t.Fatalf("release developer tools = %+v, want zero config", got)
	}
}
