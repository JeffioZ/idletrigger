package tray

import (
	"strings"
	"testing"

	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/power"
)

func TestNoSleepRequestSources(t *testing.T) {
	cfg := config.DefaultConfig()
	if noSleepRequested(cfg, false) {
		t.Fatal("unexpected request")
	}
	if !noSleepRequested(cfg, true) {
		t.Fatal("process request was ignored")
	}
	cfg.NoSleepEnabled = true
	if !noSleepRequested(cfg, false) {
		t.Fatal("user request was ignored")
	}
}

func TestBatteryPolicyBlocksAndRestores(t *testing.T) {
	cfg := config.DefaultConfig()
	battery := power.Status{Battery: true, Percent: 80, Valid: true}
	if !batteryPolicyBlocks(cfg, battery) {
		t.Fatal("battery policy should block by default")
	}
	cfg.NoSleepOnBattery = true
	if batteryPolicyBlocks(cfg, battery) {
		t.Fatal("battery policy should allow an adequately charged battery")
	}
	battery.Percent = 10
	if !batteryPolicyBlocks(cfg, battery) {
		t.Fatal("low battery threshold was ignored")
	}
	battery.ACLine = true
	if batteryPolicyBlocks(cfg, battery) {
		t.Fatal("AC power should clear the battery block")
	}
}

func TestStatusLineUsesLocalizedPunctuation(t *testing.T) {
	state := trayState{lang: "zh-CN"}
	if got := state.statusLine("status_power", "交流电源"); got != "电源：交流电源" {
		t.Fatalf("localized status line = %q", got)
	}
}

func TestTooltipStaysWithinTrayLimit(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en"} {
		cfg := config.DefaultConfig()
		cfg.Language = lang
		cfg.IdleTimeoutMinutes = 30
		cfg.IdleAction = config.ActionSleep
		cfg.ThemeSwitchEnabled = true
		cfg.ThemeMode = "sunrise"
		cfg.HotkeysEnabled = true
		cfg.AutostartEnabled = true
		cfg.LoggingEnabled = true
		state := trayState{cfg: cfg, lang: lang}

		got := state.buildTooltip()
		if strings.Contains(got, "…") {
			t.Fatalf("tooltip should not be hard-truncated for %s: %q", lang, got)
		}
		if strings.Contains(got, "·") {
			t.Fatalf("tooltip should avoid separator truncation risk for %s: %q", lang, got)
		}
		if hasDanglingTooltipSuffix(got) {
			t.Fatalf("tooltip has dangling suffix for %s: %q", lang, got)
		}
		if length := len([]rune(got)); length > 120 {
			t.Fatalf("tooltip is too long for %s: %d %q", lang, length, got)
		}
	}
}

func TestTooltipKeepsOnlyOperationalStates(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Language = "zh-CN"
	cfg.ThemeSwitchEnabled = false
	cfg.HotkeysEnabled = true
	cfg.AutostartEnabled = true
	cfg.LoggingEnabled = true
	state := trayState{cfg: cfg, lang: "zh-CN"}

	got := state.buildTooltip()
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("tooltip should keep status lines visible: %q", got)
	}
	if lines[0] != "IdleTrigger" {
		t.Fatalf("tooltip title = %q, want IdleTrigger", lines[0])
	}
	if !strings.Contains(got, "主题：关") {
		t.Fatalf("theme should remain visible: %q", got)
	}
	for _, omitted := range []string{"热键", "自启", "日志"} {
		if strings.Contains(got, omitted) {
			t.Fatalf("non-operational setting %q should not be in tooltip: %q", omitted, got)
		}
	}
}

func TestTooltipTitleAddsReleaseVersion(t *testing.T) {
	if got := tooltipTitle("1.3.5"); got != "IdleTrigger v1.3.5" {
		t.Fatalf("release tooltip title = %q", got)
	}
	if got := tooltipTitle("dev"); got != "IdleTrigger" {
		t.Fatalf("development tooltip title = %q", got)
	}
}

func TestIdleSuspendedByProcessKeepAwake(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Language = "zh-CN"
	cfg.ProcessWatchEnabled = true
	state := trayState{cfg: cfg, lang: "zh-CN", processNoSleep: true}
	if !state.idleSuspended() {
		t.Fatal("process keep-awake should suspend the configured idle monitor")
	}
	if got := state.buildTooltip(); !strings.Contains(got, "空闲监测：已暂停") {
		t.Fatalf("tooltip should expose idle suspension: %q", got)
	}
}

func hasDanglingTooltipSuffix(value string) bool {
	return strings.HasSuffix(value, " ") ||
		strings.HasSuffix(value, "·") ||
		strings.HasSuffix(value, ":") ||
		strings.HasSuffix(value, "：") ||
		strings.HasSuffix(value, "/")
}
