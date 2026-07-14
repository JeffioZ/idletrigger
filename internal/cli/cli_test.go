package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/JeffioZ/idletrigger/internal/version"
)

func TestRunWithoutArgsPrintsUsage(t *testing.T) {
	output := captureStdout(t, func() {
		withArgs(t, []string{"IdleTrigger"}, func() {
			Run("en")
		})
	})

	if !strings.Contains(output, "Usage: IdleTrigger <command>") {
		t.Fatalf("usage output missing command header:\n%s", output)
	}
	if !strings.Contains(output, "config:reload") {
		t.Fatalf("usage output missing config reload command:\n%s", output)
	}
}

func TestRunVersionExpandsVersion(t *testing.T) {
	original := version.Value
	version.Value = "test-version"
	t.Cleanup(func() { version.Value = original })

	output := captureStdout(t, func() {
		withArgs(t, []string{"IdleTrigger", "version"}, func() {
			Run("en")
		})
	})

	if strings.TrimSpace(output) != "IdleTrigger test-version" {
		t.Fatalf("unexpected version output: %q", output)
	}
}

func TestRunHelpUsesRequestedLanguage(t *testing.T) {
	output := captureStdout(t, func() {
		withArgs(t, []string{"IdleTrigger", "--help"}, func() {
			Run("zh-CN")
		})
	})

	if !strings.Contains(output, "用法：IdleTrigger <命令>") {
		t.Fatalf("Chinese help output missing usage header:\n%s", output)
	}
}

func withArgs(t *testing.T, args []string, fn func()) {
	t.Helper()
	original := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = original })
	fn()
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = original
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}
