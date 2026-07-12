//go:build devtools

package devtools

import (
	"strings"
	"testing"
)

func TestLoadRequiresMasterSwitch(t *testing.T) {
	t.Setenv(idleMonitorEnv, "10")
	t.Setenv(inputTraceEnv, "1")
	got := Load()
	if got.Enabled || got.IdleMonitorEnabled() || got.InputTrace || !got.ForceLog {
		t.Fatalf("master-switch result = %+v", got)
	}
	if !containsNotice(got.Notices, "must be exactly 1") {
		t.Fatalf("missing master-switch notice: %v", got.Notices)
	}
}

func TestLoadRejectsInvalidMasterSwitch(t *testing.T) {
	t.Setenv(masterEnv, "true")
	got := Load()
	if got.Enabled || !got.ForceLog || !containsNotice(got.Notices, "master switch ignored") {
		t.Fatalf("invalid master switch = %+v", got)
	}
}

func TestLoadAcceptsCombinedModesAndSafeIdleRange(t *testing.T) {
	t.Setenv(masterEnv, "1")
	t.Setenv(logEnv, "1")
	t.Setenv(idleMonitorEnv, "10")
	t.Setenv(inputTraceEnv, "1")
	t.Setenv(capturePanelEnv, "1")
	t.Setenv(warningEnv, "1")
	got := Load()
	if !got.Enabled || !got.ForceLog || got.IdleMonitorSeconds != 10 || !got.InputTrace || !got.CapturePanel || !got.WarningPreview {
		t.Fatalf("combined modes = %+v", got)
	}
}

func TestInputTraceForcesLogging(t *testing.T) {
	t.Setenv(masterEnv, "1")
	t.Setenv(inputTraceEnv, "1")
	got := Load()
	if !got.InputTrace || !got.ForceLog {
		t.Fatalf("input trace must force logging: %+v", got)
	}
}

func TestLoadRejectsInvalidAndLegacyVariables(t *testing.T) {
	t.Setenv(masterEnv, "1")
	t.Setenv(idleMonitorEnv, "9")
	t.Setenv(inputTraceEnv, "yes")
	t.Setenv("IDLETRIGGER_IDLE_TEST_SECONDS", "10")
	got := Load()
	if got.IdleMonitorEnabled() || got.InputTrace || !got.ForceLog {
		t.Fatalf("invalid modes = %+v", got)
	}
	for _, want := range []string{"idle-monitor value ignored", "boolean ignored", "deprecated variable ignored"} {
		if !containsNotice(got.Notices, want) {
			t.Fatalf("missing %q in notices: %v", want, got.Notices)
		}
	}
}

func containsNotice(notices []string, want string) bool {
	for _, notice := range notices {
		if strings.Contains(notice, want) {
			return true
		}
	}
	return false
}
