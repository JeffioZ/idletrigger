package tray

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/devtools"
	"github.com/JeffioZ/idletrigger/internal/power"
)

func TestOpenWithShellBuildsNativeArguments(t *testing.T) {
	previous := executeShell
	t.Cleanup(func() { executeShell = previous })

	var gotFile, gotArgs string
	executeShell = func(_ windows.Handle, _ *uint16, file, args, _ *uint16, _ int32) error {
		gotFile = windows.UTF16PtrToString(file)
		if args != nil {
			gotArgs = windows.UTF16PtrToString(args)
		}
		return nil
	}

	path := `C:\Program Files\IdleTrigger\IdleTrigger.toml`
	if err := openWithShell("notepad.exe", windows.EscapeArg(path)); err != nil {
		t.Fatalf("openWithShell: %v", err)
	}
	if gotFile != "notepad.exe" || gotArgs != `"C:\Program Files\IdleTrigger\IdleTrigger.toml"` {
		t.Fatalf("shell launch = %q %q", gotFile, gotArgs)
	}
}

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

func TestDeveloperIdleMonitorUsesSafeRuntimeSettings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IdleTimeoutMinutes = 0
	cfg.IdleAction = config.ActionShutdown
	state := trayState{
		cfg:      cfg,
		devtools: devtools.Config{Enabled: true, IdleMonitorSeconds: 10},
	}
	if !state.idleMonitorRequested() {
		t.Fatal("developer idle monitor should run even when config monitor is disabled")
	}
	threshold, warning, action, active := state.effectiveIdleMonitorSettings()
	if !active || threshold != 10*time.Second || warning != 5*time.Second || action != config.ActionLock {
		t.Fatalf("developer idle settings = %s/%s/%s/%v", threshold, warning, action, active)
	}
	if state.cfg.IdleTimeoutMinutes != 0 || state.cfg.IdleAction != config.ActionShutdown {
		t.Fatalf("developer idle monitor changed config: %+v", state.cfg)
	}
}

func TestDeveloperIdleMonitorStillRespectsStayAwakeMutualExclusion(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.NoSleepEnabled = true
	state := trayState{
		cfg:      cfg,
		devtools: devtools.Config{Enabled: true, IdleMonitorSeconds: 10},
	}
	if !state.idleSuspended() {
		t.Fatal("developer idle monitor must remain suspended by Stay Awake")
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
	battery.Percent = 0
	if !batteryPolicyBlocks(cfg, battery) {
		t.Fatal("empty battery should remain below the safety threshold")
	}
	battery.ACLine = true
	if batteryPolicyBlocks(cfg, battery) {
		t.Fatal("AC power should clear the battery block")
	}
}

func TestRuntimeModeConfigTransitions(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProcessWatchEnabled = true
	cfg.ProcessWatchList = []string{"obs64.exe"}
	setNoSleepConfig(&cfg, true, true)
	if !cfg.NoSleepEnabled || !cfg.KeepScreenOn || cfg.IdleTimeoutMinutes != 0 || !shouldRunProcessWatcher(cfg) {
		t.Fatalf("Stay Awake transition = %+v", cfg)
	}

	setIdleTimeoutConfig(&cfg, 90)
	if cfg.NoSleepEnabled || cfg.IdleTimeoutMinutes != 90 || shouldRunProcessWatcher(cfg) {
		t.Fatalf("idle-monitor transition = %+v", cfg)
	}
}

func TestActionAvailability(t *testing.T) {
	tests := []struct {
		action config.Action
		caps   power.Capabilities
		want   bool
	}{
		{config.ActionSleep, power.Capabilities{}, false},
		{config.ActionSleep, power.Capabilities{SleepAvailable: true}, true},
		{config.ActionHibernate, power.Capabilities{}, false},
		{config.ActionHibernate, power.Capabilities{HibernateAvailable: true}, true},
		{config.ActionShutdown, power.Capabilities{}, true},
		{config.ActionLock, power.Capabilities{}, true},
	}
	for _, tt := range tests {
		if got := actionAvailable(tt.action, tt.caps); got != tt.want {
			t.Errorf("actionAvailable(%q, %+v) = %v, want %v", tt.action, tt.caps, got, tt.want)
		}
	}
}

func TestIPLocationLookupEligibility(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ThemeMode = "sunrise"
	cfg.ThemeIPLocationEnabled = true
	if !ipLocationLookupEnabled(cfg) {
		t.Fatal("IP lookup should be enabled for sunrise mode without coordinates")
	}
	cfg.ThemeLatitude = 31.2
	if ipLocationLookupEnabled(cfg) {
		t.Fatal("manual coordinates should disable IP lookup")
	}
	cfg.ThemeLatitude = 0
	cfg.ThemeMode = "fixed"
	if ipLocationLookupEnabled(cfg) {
		t.Fatal("fixed mode should disable IP lookup")
	}
	cfg.ThemeMode = "sunrise"
	cfg.ThemeIPLocationEnabled = false
	if ipLocationLookupEnabled(cfg) {
		t.Fatal("disabled option should disable IP lookup")
	}
}

func TestIPLocationRetryIsLimitedToOncePerCycle(t *testing.T) {
	state := &trayState{ipLocationGeneration: 7}
	state.handleIPLocationFailure(7)
	if state.ipLocationRetry == nil {
		t.Fatal("first failure did not schedule a retry")
	}
	state.cancelIPLocationRetry()
	state.ipLocationRetried = true
	state.handleIPLocationFailure(7)
	if state.ipLocationRetry != nil {
		t.Fatal("second failure scheduled another retry")
	}
	state.ipLocationRetried = false
	state.handleIPLocationFailure(6)
	if state.ipLocationRetry != nil {
		t.Fatal("stale lookup generation scheduled a retry")
	}
}

func TestIPLocationConfigSyncDoesNotResetActiveCycle(t *testing.T) {
	state := &trayState{cfg: config.DefaultConfig(), ipLocationGeneration: 3, ipLocationRetried: true}
	state.cfg.ThemeMode = "sunrise"
	state.cfg.ThemeIPLocationEnabled = true
	state.syncIPLocationCycle(true)
	if state.ipLocationGeneration != 3 || !state.ipLocationRetried {
		t.Fatalf("unchanged eligible config reset cycle: generation=%d retried=%v", state.ipLocationGeneration, state.ipLocationRetried)
	}

	state.cfg.ThemeIPLocationEnabled = false
	state.syncIPLocationCycle(true)
	if state.ipLocationGeneration != 4 || state.ipLocationRetried {
		t.Fatalf("disabled config did not stop cycle: generation=%d retried=%v", state.ipLocationGeneration, state.ipLocationRetried)
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
