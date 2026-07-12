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
	if noSleepRequested(cfg, true) {
		t.Fatal("process request should not enable Stay Awake by itself")
	}
	cfg.NoSleepEnabled = true
	if !noSleepRequested(cfg, false) {
		t.Fatal("user request was ignored")
	}
	cfg.ProcessWatchEnabled = true
	if !noSleepRequested(cfg, false) {
		t.Fatal("empty process list should not block Stay Awake")
	}
	cfg.ProcessWatchList = []string{"obs64.exe"}
	if noSleepRequested(cfg, false) {
		t.Fatal("configured process watch should wait for a matching process")
	}
	if !noSleepRequested(cfg, true) {
		t.Fatal("matching process should allow enabled Stay Awake")
	}
}

func TestBatteryLoopIsLazy(t *testing.T) {
	s := &trayState{
		cfg:     config.DefaultConfig(),
		stateCh: make(chan stateRequest, 1),
	}
	s.syncBatteryLoop()
	if s.batteryStop != nil {
		t.Fatal("battery loop started with all battery-aware features disabled")
	}

	s.cfg.NoSleepEnabled = true
	s.syncBatteryLoop()
	if s.batteryStop == nil {
		t.Fatal("battery loop did not start for Stay Awake")
	}

	s.cfg.NoSleepEnabled = false
	s.syncBatteryLoop()
	if s.batteryStop != nil {
		t.Fatal("battery loop did not stop after Stay Awake was disabled")
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

func TestProcessMatchDoesNotSuspendIdleWhenStayAwakeIsOff(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Language = "zh-CN"
	cfg.IdleTimeoutMinutes = 30
	cfg.ProcessWatchEnabled = true
	cfg.ProcessWatchList = []string{"obs64.exe"}
	state := trayState{cfg: cfg, lang: "zh-CN", processNoSleep: true}
	if state.idleSuspended() {
		t.Fatal("process match should not suspend idle monitoring when Stay Awake is off")
	}
	if got := state.monitorStatusText(); got != "已禁用" {
		t.Fatalf("monitor status = %q, want disabled", got)
	}
}

func TestIdleSuspendedByEffectiveKeepAwake(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Language = "zh-CN"
	cfg.NoSleepEnabled = true
	cfg.IdleTimeoutMinutes = 30
	state := trayState{cfg: cfg, lang: "zh-CN"}
	if !state.idleSuspended() {
		t.Fatal("effective keep-awake should suspend the configured idle monitor")
	}
	if got := state.buildTooltip(); !strings.Contains(got, "空闲监测：已暂停") {
		t.Fatalf("tooltip should expose idle suspension: %q", got)
	}
	if got := state.monitorStatusText(); got != "保持唤醒时暂停" {
		t.Fatalf("monitor status = %q, want paused by Stay Awake", got)
	}
}

func TestProcessWatchHelpersTreatEmptyListAsNormal(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.NoSleepEnabled = true
	cfg.ProcessWatchEnabled = true
	cfg.ProcessWatchList = []string{"", " OBS64.exe ", "obs64.exe", "Code.exe"}
	got := effectiveProcessWatchList(cfg)
	if strings.Join(got, ",") != "OBS64.exe,Code.exe" {
		t.Fatalf("effective list = %v", got)
	}
	cfg.ProcessWatchList = nil
	if !noSleepRequested(cfg, false) {
		t.Fatal("empty process list should not disable Stay Awake")
	}
	if shouldRunProcessWatcher(cfg) {
		t.Fatal("empty process list should not start watcher")
	}
}

func hasDanglingTooltipSuffix(value string) bool {
	return strings.HasSuffix(value, " ") ||
		strings.HasSuffix(value, "·") ||
		strings.HasSuffix(value, ":") ||
		strings.HasSuffix(value, "：") ||
		strings.HasSuffix(value, "/")
}
