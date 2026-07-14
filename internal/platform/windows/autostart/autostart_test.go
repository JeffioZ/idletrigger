package autostart

import "testing"

func TestCommandLineQuotesExecutableWithoutEscapingQuotes(t *testing.T) {
	got := commandLine("C:\\Program Files\\IdleTrigger\\IdleTrigger-x64.exe")
	want := "\"C:\\Program Files\\IdleTrigger\\IdleTrigger-x64.exe\" --minimized"
	if got != want {
		t.Fatalf("commandLine() = %q, want %q", got, want)
	}
}

func TestCommandLineChangesWhenExecutablePathChanges(t *testing.T) {
	old := commandLine("C:\\Old\\IdleTrigger-x64.exe")
	current := commandLine("D:\\Apps\\IdleTrigger-x64.exe")
	if old == current {
		t.Fatal("commandLine should include the executable path")
	}
}
