//go:build devtools

package cli

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDevtoolsUsageIncludesDiagnostics(t *testing.T) {
	if !strings.Contains(usage("zh-CN"), "diagnostics  查看原始空闲计时") {
		t.Fatal("devtools usage is missing diagnostics")
	}
}

func TestRunDiagnosticsIdlePrintsSnapshot(t *testing.T) {
	output := captureStdout(t, func() {
		withArgs(t, []string{"IdleTrigger", "diagnostics", "idle"}, func() {
			Run("en")
		})
	})

	if !strings.Contains(output, "idle_diagnostics tick_now=") {
		t.Fatalf("diagnostics output missing snapshot:\n%s", output)
	}
}

func TestRunDiagnosticsInvalidArgsUsesLocalizedUsage(t *testing.T) {
	if lang := os.Getenv("IDLETRIGGER_TEST_DIAGNOSTICS_CHILD_LANG"); lang != "" {
		withArgs(t, []string{"IdleTrigger", "diagnostics"}, func() {
			Run(lang)
		})
		return
	}

	for _, tt := range []struct {
		lang string
		want string
	}{
		{"en", "Usage: IdleTrigger diagnostics idle [--watch]"},
		{"zh-CN", "用法：IdleTrigger diagnostics idle [--watch]"},
	} {
		t.Run(tt.lang, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestRunDiagnosticsInvalidArgsUsesLocalizedUsage$")
			cmd.Env = append(os.Environ(), "IDLETRIGGER_TEST_DIAGNOSTICS_CHILD_LANG="+tt.lang)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("diagnostics with missing subcommand succeeded")
			}
			if !strings.Contains(string(output), tt.want) {
				t.Fatalf("localized diagnostics usage missing %q:\n%s", tt.want, output)
			}
		})
	}
}
