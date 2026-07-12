//go:build devtools

package devtools

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	masterEnv       = "IDLETRIGGER_DEVTOOLS"
	logEnv          = "IDLETRIGGER_DEVTOOLS_LOG"
	idleMonitorEnv  = "IDLETRIGGER_DEVTOOLS_IDLE_MONITOR_SECONDS"
	inputTraceEnv   = "IDLETRIGGER_DEVTOOLS_INPUT_TRACE"
	capturePanelEnv = "IDLETRIGGER_DEVTOOLS_CAPTURE_PANEL"
	warningEnv      = "IDLETRIGGER_DEVTOOLS_WARNING_PREVIEW"

	minIdleMonitorSeconds = 10
	maxIdleMonitorSeconds = 600
)

var legacyEnvs = []string{
	"IDLETRIGGER_DEBUG_LOG",
	"IDLETRIGGER_IDLE_TEST_SECONDS",
	"IDLETRIGGER_INPUT_DIAGNOSTICS",
	"IDLETRIGGER_CAPTURE_MODE",
	"IDLETRIGGER_DEV",
}

// Load resolves developer-tool environment variables once. All feature
// variables require the explicit master switch and are ignored otherwise.
func Load() Config {
	c := Config{}
	present := make([]string, 0, 5)
	for _, name := range []string{logEnv, idleMonitorEnv, inputTraceEnv, capturePanelEnv, warningEnv} {
		if _, ok := os.LookupEnv(name); ok {
			present = append(present, name)
		}
	}
	for _, name := range legacyEnvs {
		if _, ok := os.LookupEnv(name); ok {
			c.Notices = append(c.Notices, fmt.Sprintf("Developer tools deprecated variable ignored: %s", name))
		}
	}

	masterValue, masterPresent := os.LookupEnv(masterEnv)
	if masterValue != "1" {
		if masterPresent {
			c.Notices = append(c.Notices, fmt.Sprintf("Developer tools master switch ignored: %s=%q (expected 1)", masterEnv, masterValue))
		}
		if len(present) > 0 {
			c.Notices = append(c.Notices, "Developer tools variables ignored: IDLETRIGGER_DEVTOOLS must be exactly 1")
		}
		c.ForceLog = len(c.Notices) > 0
		return c
	}

	c.Enabled = true
	c.InputTrace = boolEnv(inputTraceEnv, &c.Notices)
	c.CapturePanel = boolEnv(capturePanelEnv, &c.Notices)
	c.WarningPreview = boolEnv(warningEnv, &c.Notices)
	forceLog := boolEnv(logEnv, &c.Notices)
	if raw, ok := os.LookupEnv(idleMonitorEnv); ok {
		seconds, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || seconds < minIdleMonitorSeconds || seconds > maxIdleMonitorSeconds {
			c.Notices = append(c.Notices, fmt.Sprintf("Developer tools idle-monitor value ignored: %s=%q (allowed %d..%d)", idleMonitorEnv, raw, minIdleMonitorSeconds, maxIdleMonitorSeconds))
		} else {
			c.IdleMonitorSeconds = seconds
		}
	}
	// Every active developer mode emits a startup record. Input trace always
	// forces logging because its only output is the debug log.
	c.ForceLog = forceLog || c.InputTrace || c.IdleMonitorEnabled() || c.CapturePanel || c.WarningPreview || len(c.Notices) > 0
	return c
}

func boolEnv(name string, notices *[]string) bool {
	raw, ok := os.LookupEnv(name)
	if !ok || raw == "" {
		return false
	}
	if raw == "1" {
		return true
	}
	*notices = append(*notices, fmt.Sprintf("Developer tools boolean ignored: %s=%q (expected 1)", name, raw))
	return false
}
