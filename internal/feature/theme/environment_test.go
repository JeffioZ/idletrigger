package theme

import (
	"errors"
	"testing"
	"time"
	"unsafe"
)

func TestWindowCoversMonitorAllowsDPIAdjustedEdgeTolerance(t *testing.T) {
	monitor := windowRect{left: 0, top: 0, right: 2560, bottom: 1440}
	for _, test := range []struct {
		name      string
		window    windowRect
		tolerance int32
		want      bool
	}{
		{"exact", monitor, 2, true},
		{"within tolerance", windowRect{left: 3, top: 2, right: 2557, bottom: 1438}, 3, true},
		{"outside tolerance", windowRect{left: 4, top: 0, right: 2560, bottom: 1440}, 3, false},
		{"maximized work area", windowRect{left: 0, top: 0, right: 2560, bottom: 1400}, 3, false},
		{"empty window", windowRect{}, 3, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := windowCoversMonitor(test.window, monitor, test.tolerance); got != test.want {
				t.Fatalf("windowCoversMonitor(%+v) = %v, want %v", test.window, got, test.want)
			}
		})
	}
}

func TestExcludedFullscreenWindowClass(t *testing.T) {
	for _, className := range []string{"Progman", "WorkerW", "Shell_TrayWnd", "SHELL_SECONDARYTRAYWND"} {
		if !excludedFullscreenWindowClass(className) {
			t.Errorf("shell class %q was not excluded", className)
		}
	}
	for _, className := range []string{"Chrome_WidgetWin_1", "SDL_app", "UnrealWindow", ""} {
		if excludedFullscreenWindowClass(className) {
			t.Errorf("application class %q was excluded", className)
		}
	}
}

func TestThemeSwitchPauseReasonForNotificationState(t *testing.T) {
	for state, want := range map[uint32]ThemeSwitchPauseReason{
		queryUserNotificationStateBusy:                 ThemeSwitchPauseFullscreen,
		queryUserNotificationStateRunningD3DFullscreen: ThemeSwitchPauseDirect3D,
		queryUserNotificationStatePresentationMode:     ThemeSwitchPausePresentation,
		0: ThemeSwitchPauseNone,
		1: ThemeSwitchPauseNone,
		5: ThemeSwitchPauseNone,
		6: ThemeSwitchPauseNone,
	} {
		if got := themeSwitchPauseReasonForNotificationState(state); got != want {
			t.Errorf("state %d = %q, want %q", state, got, want)
		}
	}
}

func TestForegroundProcess3DUsageMatchesExactPIDAndRenderingEngines(t *testing.T) {
	samples := []gpuCounterSample{
		{name: "pid_123_luid_0x00000000_0x00001234_phys_0_eng_0_engtype_3D", usage: 24.5},
		{name: "pid_123_luid_0x00000000_0x00001234_phys_0_eng_1_engtype_Copy", usage: 99},
		{name: "pid_123_luid_0x00000000_0x00001234_phys_0_eng_2_engtype_Graphics", usage: 31},
		{name: "pid_1234_luid_0x00000000_0x00001234_phys_0_eng_0_engtype_3D", usage: 88},
		{name: "pid_12_luid_0x00000000_0x00001234_phys_0_eng_0_engtype_3D", usage: 77},
	}
	if got := foregroundProcess3DUsage(samples, 123); got != 55.5 {
		t.Fatalf("foregroundProcess3DUsage = %.1f, want 55.5", got)
	}
}

func TestPDHFormattedItemLayoutMatchesWindowsABI(t *testing.T) {
	var item pdhFormattedItem
	if got := unsafe.Sizeof(item); got != 24 {
		t.Fatalf("PDH item size = %d, want 24", got)
	}
	if got := unsafe.Offsetof(item.status); got != 8 {
		t.Fatalf("PDH status offset = %d, want 8", got)
	}
	if got := unsafe.Offsetof(item.value); got != 16 {
		t.Fatalf("PDH value offset = %d, want 16", got)
	}
}

func TestGPUActivitySampleWaitCanBeCanceled(t *testing.T) {
	cancel := make(chan struct{})
	close(cancel)
	started := time.Now()
	err := waitForThemeEnvironmentSample(cancel, time.Hour)
	if !errors.Is(err, errThemeEnvironmentCheckCanceled) {
		t.Fatalf("wait error = %v, want cancellation", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("canceled wait took %s", elapsed)
	}
}

func TestSchedulerPauseDetectionPreventsThemeSwitch(t *testing.T) {
	s := NewScheduler("fixed", "07:00", "19:00", 0, 0, true, false)
	calls := 0
	s.pauseCheck = func(<-chan struct{}) (ThemeSwitchPauseReason, error) {
		calls++
		return ThemeSwitchPauseForegroundGPU, nil
	}
	s.switchIfAllowed("schedule", ModeDark, nil)
	if calls != 1 {
		t.Fatalf("pause detector calls = %d, want 1", calls)
	}
}
