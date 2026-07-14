// Package tray manages the Windows system-tray icon and context menu.
package tray

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/actions"
	"github.com/JeffioZ/idletrigger/internal/autostart"
	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/devtools"
	"github.com/JeffioZ/idletrigger/internal/dialog"
	"github.com/JeffioZ/idletrigger/internal/hotkey"
	"github.com/JeffioZ/idletrigger/internal/i18n"
	"github.com/JeffioZ/idletrigger/internal/idlewarning"
	"github.com/JeffioZ/idletrigger/internal/ipc"
	mylog "github.com/JeffioZ/idletrigger/internal/log"
	"github.com/JeffioZ/idletrigger/internal/monitor"
	"github.com/JeffioZ/idletrigger/internal/nosleep"
	"github.com/JeffioZ/idletrigger/internal/popup"
	"github.com/JeffioZ/idletrigger/internal/power"
	"github.com/JeffioZ/idletrigger/internal/processwatcher"
	"github.com/JeffioZ/idletrigger/internal/systray"
	"github.com/JeffioZ/idletrigger/internal/themeswitch"
	"github.com/JeffioZ/idletrigger/internal/version"
	"github.com/JeffioZ/idletrigger/internal/winresource"
)

const projectHomeURL = "https://github.com/JeffioZ/idletrigger"

var executeShell = windows.ShellExecute

func openWithShell(target, arguments string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	var args *uint16
	if arguments != "" {
		args, err = windows.UTF16PtrFromString(arguments)
		if err != nil {
			return err
		}
	}
	const swShowNormal = 1
	return executeShell(0, verb, file, args, nil, swShowNormal)
}

type Callbacks struct {
	OnConfigChanged  func(config.Config)
	ShowPopupOnStart bool
	DeveloperTools   devtools.Config
}

type trayState struct {
	cfg       config.Config
	lang      string
	callbacks Callbacks
	devtools  devtools.Config
	stateCh   chan stateRequest

	mon *monitor.Monitor

	hotkeyMgr       *hotkey.Manager
	procWatch       *processwatcher.Watcher
	themeSched      *themeswitch.Scheduler
	batteryStop     chan struct{}
	batteryDone     chan struct{}
	processNoSleep  bool
	batteryBlocked  bool
	trayThemeDark   bool
	menuOpen        *systray.MenuItem
	menuExit        *systray.MenuItem
	selfConfigMod   atomic.Int64
	selfConfigWrite atomic.Bool
}

type stateRequest struct {
	fn     func() string
	result chan string
}

// Run starts the system-tray loop. Blocks until Exit.
func Run(cfg config.Config, cbs Callbacks) {
	systray.SetErrorHandler(func(format string, args ...interface{}) {
		mylog.Info("Systray: %s", fmt.Sprintf(format, args...))
	})

	s := &trayState{
		cfg:       cfg,
		lang:      cfg.Language,
		callbacks: cbs,
		devtools:  cbs.DeveloperTools,
		stateCh:   make(chan stateRequest, 64),
	}

	onReady := func() {
		s.menuOpen = systray.AddMenuItem(i18n.T(s.lang, "menu_open_panel"), "")
		s.menuExit = systray.AddMenuItem(i18n.T(s.lang, "menu_exit"), "")
		go func() {
			for range s.menuOpen.ClickedCh {
				systray.Post(s.showPopup)
			}
		}()
		go func() {
			for range s.menuExit.ClickedCh {
				popup.Destroy()
				systray.Quit()
			}
		}()

		if enabled, updated, err := autostart.EnsureCurrent(); err == nil {
			s.cfg.AutostartEnabled = enabled
			if updated {
				mylog.Info("Autostart entry updated to current executable path")
			}
		} else {
			mylog.Info("Autostart check failed: %v", err)
		}

		// Config compatibility: if both NoSleep and idle monitor are enabled,
		// resolve the conflict — NoSleep takes priority.
		if s.cfg.NoSleepEnabled && s.cfg.IdleTimeoutMinutes > 0 {
			s.cfg.IdleTimeoutMinutes = 0
		}
		s.batteryBlocked = batteryPolicyBlocks(s.cfg, power.GetStatus())
		s.reconcileRuntime()

		s.updateIcon()

		// Hotkeys
		if s.cfg.HotkeysEnabled {
			s.startHotkeys()
		}

		// Process watcher
		s.syncProcessWatcher()
		if s.cfg.ThemeSwitchEnabled {
			s.startThemeScheduler()
		}
		s.refreshIPLocationInBackground()

		go s.stateLoop()
		go func() {
			if err := ipc.Server(s.handleIPC); err != nil {
				mylog.Info("IPC server stopped: %v", err)
			}
		}()
		s.syncBatteryLoop()
		go s.watchConfig()
		if cbs.ShowPopupOnStart {
			systray.Post(s.showPopup)
		}

		systray.OnLeftClick = func() { s.showPopup() }
		systray.OnPowerChange = func() {
			s.post(func() { s.refreshBatteryPolicy() })
		}
		systray.OnThemeChange = func() {
			s.post(s.refreshTrayThemeIcon)
		}
	}

	onExit := func() {
		s.call(func() string {
			s.stopMonitor()
			s.stopHotkeys()
			s.stopProcessWatcher()
			s.stopThemeScheduler()
			s.stopBatteryLoop()
			nosleep.Disable()
			return ""
		})
	}

	systray.Run(onReady, onExit)
}

func (s *trayState) stateLoop() {
	for req := range s.stateCh {
		result := req.fn()
		if req.result != nil {
			req.result <- result
		}
	}
}

func (s *trayState) post(fn func()) {
	s.stateCh <- stateRequest{fn: func() string {
		fn()
		return ""
	}}
}

func (s *trayState) call(fn func() string) string {
	result := make(chan string, 1)
	s.stateCh <- stateRequest{fn: fn, result: result}
	return <-result
}

// ---- state sync -------------------------------------------------------

// ---- icon state -------------------------------------------------------

func (s *trayState) updateIcon() {
	darkTheme := themeswitch.Current() == themeswitch.ModeDark
	if darkTheme {
		systray.SetIconResource(winresource.TrayLightIconID)
	} else {
		systray.SetIconResource(winresource.TrayDarkIconID)
	}
	s.trayThemeDark = darkTheme
	systray.SetTooltip(s.buildTooltip())
}

func (s *trayState) refreshTrayThemeIcon() {
	if (themeswitch.Current() == themeswitch.ModeDark) != s.trayThemeDark {
		s.updateIcon()
	}
}

func (s *trayState) buildTooltip() string {
	lines := []string{tooltipTitle(version.Value)}
	lines = append(lines, s.statusLine("tooltip_nosleep", shortStatus(s.lang, nosleep.IsEnabled())))
	if s.idleSuspended() {
		lines = append(lines, s.statusLine("tooltip_idle", i18n.T(s.lang, "status_paused")))
	} else if s.devtools.IdleMonitorEnabled() && s.mon != nil {
		lines = append(lines, s.statusLine("tooltip_idle", s.developerIdleMonitorStatus()))
	} else if s.cfg.IdleTimeoutMinutes > 0 {
		lines = append(lines, s.statusLine("tooltip_idle", fmt.Sprintf("%d%s %s", s.cfg.IdleTimeoutMinutes, shortMinuteUnit(s.lang), i18n.T(s.lang, actionTranslationKey(s.cfg.IdleAction)))))
	} else {
		lines = append(lines, s.statusLine("tooltip_idle", shortStatus(s.lang, false)))
	}
	lines = append(lines, s.statusLine("tooltip_theme", s.themeTooltipValueShort()))
	if s.cfg.ProcessWatchEnabled || len(s.cfg.ProcessWatchList) > 0 {
		lines = append(lines, s.statusLine("tooltip_process", s.processTooltipValueShort()))
	}
	return tooltipText(lines)
}

func tooltipTitle(appVersion string) string {
	if appVersion == "" || appVersion == "dev" {
		return "IdleTrigger"
	}
	return "IdleTrigger v" + appVersion
}

func shortStatus(lang string, enabled bool) string {
	if enabled {
		return i18n.T(lang, "status_short_on")
	}
	return i18n.T(lang, "status_short_off")
}

func shortMinuteUnit(lang string) string {
	if i18n.ResolveLanguage(lang) == "zh-CN" {
		return "分"
	}
	return "m"
}

func (s *trayState) themeTooltipValueShort() string {
	if !s.cfg.ThemeSwitchEnabled {
		return i18n.T(s.lang, "status_short_off")
	}
	if schedule := s.themeScheduleText(false); schedule != "" {
		return i18n.T(s.lang, "status_short_on") + " " + compactThemeSchedule(schedule)
	}
	return i18n.T(s.lang, "status_short_on")
}

func (s *trayState) processTooltipValueShort() string {
	if len(effectiveProcessWatchList(s.cfg)) == 0 {
		return i18n.T(s.lang, "status_process_not_configured")
	}
	if !s.cfg.ProcessWatchEnabled {
		return shortStatus(s.lang, false)
	}
	if s.processNoSleep {
		return i18n.T(s.lang, "status_process_matched")
	}
	return i18n.T(s.lang, "status_process_waiting")
}

func compactThemeSchedule(schedule string) string {
	replacer := strings.NewReplacer(
		"浅色 ", "浅",
		"深色 ", "深",
		"Light ", "L",
		"Dark ", "D",
		" / ", "/",
	)
	return replacer.Replace(schedule)
}

func (s *trayState) themeScheduleText(showSource bool) string {
	loc := themeswitch.LocationInfo{Latitude: s.cfg.ThemeLatitude, Longitude: s.cfg.ThemeLongitude, Source: themeswitch.LocationSourceConfigured}
	if s.cfg.ThemeMode == "sunrise" {
		loc = s.themeLocationInfo(false)
	}
	scheduleInfo := themeswitch.ScheduleInfoFor(s.cfg.ThemeMode, s.cfg.ThemeLightTime, s.cfg.ThemeDarkTime, loc.Latitude, loc.Longitude, time.Now())
	if !scheduleInfo.OK {
		return i18n.T(s.lang, "theme_schedule_unavailable")
	}
	formatKey := "theme_schedule_format"
	if s.cfg.ThemeMode == "sunrise" {
		formatKey = "theme_schedule_sunrise_format"
	}
	schedule := fmt.Sprintf(i18n.T(s.lang, formatKey), scheduleInfo.LightTime, scheduleInfo.DarkTime)
	if scheduleInfo.FixedFallback {
		return fmt.Sprintf(i18n.T(s.lang, "theme_schedule_fallback_format"), schedule)
	}
	if s.cfg.ThemeMode != "sunrise" || !showSource {
		return schedule
	}
	return fmt.Sprintf(i18n.T(s.lang, "theme_schedule_source_format"), schedule, s.locationSourceShort(loc))
}

func (s *trayState) locationSourceShort(loc themeswitch.LocationInfo) string {
	switch loc.Source {
	case themeswitch.LocationSourceConfigured:
		return i18n.T(s.lang, "theme_location_configured")
	case themeswitch.LocationSourceIP:
		return i18n.T(s.lang, "theme_location_ip")
	case themeswitch.LocationSourceTimezone:
		return i18n.T(s.lang, "theme_location_timezone")
	case themeswitch.LocationSourceUTCOffset:
		return i18n.T(s.lang, "theme_location_utc_offset")
	default:
		return i18n.T(s.lang, "theme_location_default")
	}
}

func (s *trayState) themeLocationInfo(blockIPLookup bool) themeswitch.LocationInfo {
	lat, lon := s.cfg.ThemeLatitude, s.cfg.ThemeLongitude
	if lat != 0 || lon != 0 {
		return themeswitch.LocationInfo{Latitude: lat, Longitude: lon, Source: themeswitch.LocationSourceConfigured}
	}
	return themeswitch.AutoLocationInfo(s.cfg.ThemeIPLocationEnabled, blockIPLookup)
}

func (s *trayState) ipLocationLabel() string {
	if !s.cfg.ThemeIPLocationEnabled {
		return ""
	}
	loc := s.themeLocationInfo(false)
	if loc.Source == themeswitch.LocationSourceIP {
		return loc.LocationLabel
	}
	return ""
}

func tooltipText(lines []string) string {
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			clean = append(clean, line)
		}
	}
	return strings.Join(clean, "\n")
}

// ---- idle monitor -----------------------------------------------------

func (s *trayState) startMonitor() {
	s.stopMonitor()
	if !s.idleMonitorRequested() {
		return
	}
	threshold, warnOffset, action, developerTest := s.effectiveIdleMonitorSettings()
	if developerTest {
		mylog.Info("Developer tools idle-monitor test active: effective_threshold=%s effective_action=lock effective_warning=%s config_unchanged=true", threshold, warnOffset)
	}
	snap, snapErr := monitor.Snapshot()
	if snapErr != nil {
		mylog.Info("Idle monitor starting: config_timeout_min=%d effective_threshold=%s warning=%s action=%s idle_snapshot_error=%v",
			s.cfg.IdleTimeoutMinutes, threshold, warnOffset, string(action), snapErr)
	} else {
		mylog.Info("Idle monitor starting: config_timeout_min=%d effective_threshold=%s warning=%s action=%s tick_now=%d tick32=%d last_input=%d raw_delta_ms=%d idle=%s",
			s.cfg.IdleTimeoutMinutes, threshold, warnOffset, string(action), snap.NowTick64, snap.NowTick32, snap.LastInputTick, snap.RawDeltaMS, snap.Idle.Round(time.Second))
	}
	mylog.Info("Idle monitor input policy: enhanced_idle_monitor=%v periodic_window=%s..%s periodic_tolerance=%s periodic_required=%d",
		s.cfg.IdleEnhancedMonitor, 20*time.Second, 2*time.Minute, 5*time.Second, 3)
	lang := s.lang
	warningSeconds := int(warnOffset / time.Second)
	processWatchEnabled := s.cfg.ProcessWatchEnabled
	processListCount := len(effectiveProcessWatchList(s.cfg))
	noSleepEnabled := s.cfg.NoSleepEnabled
	keepScreenOn := s.cfg.KeepScreenOn
	enhancedIdleMonitor := s.cfg.IdleEnhancedMonitor
	sampleLogInterval := 5 * time.Second
	if developerTest {
		sampleLogInterval = 2 * time.Second
	}
	var lastSampleLog time.Time
	var lastEffectiveIdle time.Duration
	idlewarning.SetOnDismiss(func() {
		s.post(func() {
			if s.mon != nil && s.idleMonitorRequested() {
				s.startMonitor()
			}
		})
	})

	s.mon = monitor.New(threshold, warnOffset,
		// Show a non-activating in-app warning. Legacy notification-area
		// balloons are silently suppressed on many Windows 11 configurations.
		func() {
			actName := i18n.T(lang, actionTranslationKey(action))
			title := i18n.T(lang, "idle_warning_title")
			idlewarning.SetLanguage(i18n.ResolveLanguage(lang) == "zh-CN")
			mylog.Info("Idle warning displayed: action=%s seconds=%d", action, warningSeconds)
			idlewarning.ShowCountdown(title, warningSeconds, func(remaining int) string {
				if remaining < 0 {
					remaining = 0
				}
				return fmt.Sprintf(i18n.T(lang, "msg_idle_warning"), actName, remaining)
			})
		},
		// Execute directly from the monitor goroutine. Waiting for the tray
		// state queue would make an overdue power action depend on UI work.
		func() {
			// The warning must be gone before a power action takes effect.
			systray.PostAndWait(idlewarning.Hide)
			idleFor, idleErr := monitor.IdleDuration()
			if idleErr != nil {
				mylog.Info("Idle monitor trigger reached: effective_idle=%s action=%s raw_idle_after_trigger_error=%v",
					lastEffectiveIdle.Round(time.Second), action, idleErr)
			} else {
				mylog.Info("Idle monitor trigger reached: effective_idle=%s action=%s raw_idle_after_trigger=%s",
					lastEffectiveIdle.Round(time.Second), action, idleFor.Round(time.Second))
			}
			if err := s.executeAction(action); err != nil {
				mylog.Info("Idle monitor action failed: action=%s error=%v", action, err)
				s.post(func() { s.showError(actionTranslationKey(action), err) })
				return
			}
			mylog.Info("Idle monitor action accepted: action=%s", action)
		},
		time.Second,
	)
	s.mon.SetEnhancedIdleMonitor(enhancedIdleMonitor)
	s.mon.SetOnActivity(func() { systray.Post(idlewarning.Hide) })
	s.mon.SetOnInputReset(func(reset monitor.InputReset) {
		mylog.Info("Idle monitor input reset: previous_last_input=%d last_input=%d session_idle_before_reset=%s threshold=%s warn_at=%s was_warned=%v was_triggered=%v ignored=%v reason=%s periodic_count=%d periodic_baseline=%s enhanced_idle_monitor=%v action=%s source=GetLastInputInfo",
			reset.PreviousLastInputTick, reset.LastInputTick, reset.SessionIdleBeforeReset.Round(time.Millisecond),
			reset.Threshold, reset.WarnAt, reset.WasWarned, reset.WasTriggered, reset.Ignored, reset.Reason,
			reset.PeriodicCount, reset.PeriodicBaseline.Round(time.Millisecond), enhancedIdleMonitor, action)
	})
	s.mon.SetOnSample(func(sample monitor.Sample) {
		lastEffectiveIdle = sample.Idle
		now := time.Now()
		if !lastSampleLog.IsZero() && now.Sub(lastSampleLog) < sampleLogInterval {
			return
		}
		lastSampleLog = now
		if snap, err := monitor.Snapshot(); err == nil {
			mylog.Info("Idle monitor sample: effective_idle=%s raw_idle=%s raw_delta_ms=%d threshold=%s warn_at=%s last_input=%d tick_now=%d clamped_to_start=%v warned=%v triggered=%v input_timestamp=%v enhanced_idle_monitor=%v nosleep_enabled=%v keep_screen_on=%v process_watch_enabled=%v process_list_count=%d",
				sample.Idle.Round(time.Millisecond), snap.Idle.Round(time.Millisecond), snap.RawDeltaMS,
				sample.Threshold, sample.WarnAt, sample.LastInputTick, snap.NowTick64,
				sample.StartWindowClamped, sample.Warned, sample.Triggered, sample.InputTimestampAvailable,
				enhancedIdleMonitor, noSleepEnabled, keepScreenOn, processWatchEnabled, processListCount)
		} else {
			mylog.Info("Idle monitor sample: effective_idle=%s threshold=%s warn_at=%s last_input=%d clamped_to_start=%v warned=%v triggered=%v input_timestamp=%v snapshot_error=%v",
				sample.Idle.Round(time.Millisecond), sample.Threshold, sample.WarnAt, sample.LastInputTick,
				sample.StartWindowClamped, sample.Warned, sample.Triggered, sample.InputTimestampAvailable, err)
		}
	})
	s.mon.Start()
}

func (s *trayState) stopMonitor() {
	idlewarning.SetOnDismiss(nil)
	systray.Post(idlewarning.Hide)
	if s.mon != nil {
		s.mon.Stop()
		s.mon = nil
		mylog.Info("Idle monitor stopped")
	}
}

func (s *trayState) reconcileRuntime() {
	s.syncBatteryLoop()
	wantsNoSleep := noSleepRequested(s.cfg, s.processNoSleep)
	mylog.Info("Runtime reconcile: nosleep_enabled=%v process_watch_enabled=%v process_list_count=%d process_match=%v wants_nosleep=%v battery_blocked=%v idle_timeout_min=%d monitor_running=%v",
		s.cfg.NoSleepEnabled, s.cfg.ProcessWatchEnabled, len(effectiveProcessWatchList(s.cfg)), s.processNoSleep, wantsNoSleep, s.batteryBlocked, s.cfg.IdleTimeoutMinutes, s.mon != nil)
	if wantsNoSleep && !s.batteryBlocked {
		if s.devtools.IdleMonitorEnabled() {
			mylog.Info("Developer tools idle-monitor test suppressed: Stay Awake remains mutually exclusive; config_unchanged=true")
		}
		s.stopMonitor()
		if err := nosleep.Enable(s.cfg.KeepScreenOn); err != nil {
			mylog.Info("Stay Awake enable failed: keep_screen_on=%v error=%v", s.cfg.KeepScreenOn, err)
			s.showError("menu_nosleep", err)
		} else {
			mylog.Info("Stay Awake enabled: keep_screen_on=%v", s.cfg.KeepScreenOn)
		}
		return
	}

	nosleep.Disable()
	if s.idleMonitorRequested() {
		s.startMonitor()
	} else {
		s.stopMonitor()
	}
}

func noSleepRequested(cfg config.Config, processRequested bool) bool {
	if !cfg.NoSleepEnabled {
		return false
	}
	if !cfg.ProcessWatchEnabled || len(effectiveProcessWatchList(cfg)) == 0 {
		return true
	}
	return processRequested
}

// idleSuspended reports the intentional conflict resolution between effective
// keep-awake and the idle action.
func (s *trayState) idleSuspended() bool {
	return s.idleMonitorRequested() && noSleepRequested(s.cfg, s.processNoSleep) && s.mon == nil
}

func (s *trayState) idleMonitorRequested() bool {
	return s.cfg.IdleTimeoutMinutes > 0 || s.devtools.IdleMonitorEnabled()
}

func (s *trayState) effectiveIdleMonitorSettings() (time.Duration, time.Duration, config.Action, bool) {
	if s.devtools.IdleMonitorEnabled() {
		return time.Duration(s.devtools.IdleMonitorSeconds) * time.Second, 5 * time.Second, config.ActionLock, true
	}
	return time.Duration(s.cfg.IdleTimeoutMinutes) * time.Minute, time.Duration(s.cfg.IdleWarningSeconds) * time.Second, s.cfg.IdleAction, false
}

func (s *trayState) developerIdleMonitorStatus() string {
	return fmt.Sprintf(i18n.T(s.lang, "status_monitor_test_active"), s.devtools.IdleMonitorSeconds, i18n.T(s.lang, "menu_action_lock"))
}

// ---- hotkeys ----------------------------------------------------------

func (s *trayState) startHotkeys() {
	s.stopHotkeys()
	mgr := hotkey.NewManager(hotkey.DefaultBindings(), hotkey.Callbacks{
		OnSleep: func() {
			s.post(func() {
				if err := s.executeAction(config.ActionSleep); err != nil {
					s.showError("menu_sleep", err)
				}
			})
		},
		OnLock: func() {
			s.post(func() {
				if err := actions.Lock(); err != nil {
					s.showError("menu_lock", err)
				}
			})
		},
		OnToggleNoSleep: func() { s.post(func() { s.toggleNoSleep() }) },
	})
	s.hotkeyMgr = mgr
	failed := mgr.Start()
	if len(failed) > 0 {
		s.showHotkeyConflict(failed)
	}
}

func (s *trayState) stopHotkeys() {
	if s.hotkeyMgr != nil {
		s.hotkeyMgr.Stop()
		s.hotkeyMgr = nil
	}
}

func (s *trayState) showHotkeyConflict(failed hotkey.Failed) {
	body := ""
	for _, f := range failed {
		body += "• " + f + "\n"
	}
	dialog.Warn(
		i18n.T(s.lang, "app_title"),
		i18n.T(s.lang, "msg_hotkey_conflict"),
		body,
	)
}

func (s *trayState) toggleNoSleep() {
	s.setNoSleep(!s.cfg.NoSleepEnabled, s.cfg.KeepScreenOn)
	mylog.Info("NoSleep toggled: enabled=%v keep_screen_on=%v", nosleep.IsEnabled(), nosleep.IsKeepingScreenOn())
	s.saveConfig()
}

func (s *trayState) setNoSleep(enabled, keepScreenOn bool) {
	setNoSleepConfig(&s.cfg, enabled, keepScreenOn)
	s.syncProcessWatcher()
	s.reconcileRuntime()
	s.updateIcon()
}

func (s *trayState) setIdleTimeout(minutes int) {
	setIdleTimeoutConfig(&s.cfg, minutes)
	s.syncProcessWatcher()
	s.reconcileRuntime()
	s.updateIcon()
}

func setNoSleepConfig(cfg *config.Config, enabled, keepScreenOn bool) {
	cfg.NoSleepEnabled = enabled
	if enabled {
		cfg.KeepScreenOn = keepScreenOn
		cfg.IdleTimeoutMinutes = 0
	}
}

func setIdleTimeoutConfig(cfg *config.Config, minutes int) {
	cfg.IdleTimeoutMinutes = minutes
	if minutes > 0 {
		cfg.NoSleepEnabled = false
	}
}

// ---- process watcher --------------------------------------------------

func (s *trayState) startProcessWatcher() {
	s.stopProcessWatcher()
	list := effectiveProcessWatchList(s.cfg)
	if len(list) == 0 {
		mylog.Info("Process condition inactive: process_watch_list is empty; Stay Awake is not process-limited")
		return
	}
	mylog.Info("Process watcher started: exes=%s", strings.Join(list, ","))
	s.procWatch = processwatcher.New(list,
		processwatcher.Callbacks{
			OnEnable: func() {
				s.post(func() {
					mylog.Info("Process watcher: watched app detected")
					s.processNoSleep = true
					s.reconcileRuntime()

					s.updateIcon()
				})
			},
			OnDisable: func() {
				s.post(func() {
					mylog.Info("Process watcher: no watched apps running")
					s.processNoSleep = false
					s.reconcileRuntime()

					s.updateIcon()
				})
			},
		}, 0)
	s.procWatch.Start()
}

func (s *trayState) stopProcessWatcher() {
	if s.procWatch != nil {
		s.procWatch.Stop()
		s.procWatch = nil
		mylog.Info("Process watcher stopped")
	}
	s.processNoSleep = false
}

func (s *trayState) syncProcessWatcher() {
	if shouldRunProcessWatcher(s.cfg) {
		s.startProcessWatcher()
		return
	}
	s.stopProcessWatcher()
	if s.cfg.ProcessWatchEnabled && len(effectiveProcessWatchList(s.cfg)) == 0 {
		mylog.Info("Process condition inactive at runtime: process_watch_list is empty; idle monitoring is unaffected")
	}
}

func shouldRunProcessWatcher(cfg config.Config) bool {
	return cfg.NoSleepEnabled && cfg.ProcessWatchEnabled && len(effectiveProcessWatchList(cfg)) > 0
}

func effectiveProcessWatchList(cfg config.Config) []string {
	out := make([]string, 0, len(cfg.ProcessWatchList))
	seen := make(map[string]struct{}, len(cfg.ProcessWatchList))
	for _, value := range cfg.ProcessWatchList {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

// ---- battery awareness ------------------------------------------------

// syncBatteryLoop keeps the periodic battery poll dormant when neither
// battery-aware feature is enabled. Windows power broadcasts still deliver
// immediate changes; this loop catches battery-percentage threshold crossings.
func (s *trayState) syncBatteryLoop() {
	if s.cfg.NoSleepEnabled || s.cfg.ThemeSwitchEnabled {
		if s.batteryStop != nil {
			return
		}
		s.batteryStop = make(chan struct{})
		s.batteryDone = make(chan struct{})
		go s.batteryLoop(s.batteryStop, s.batteryDone)
		return
	}
	s.stopBatteryLoop()
}

func (s *trayState) stopBatteryLoop() {
	if s.batteryStop == nil {
		return
	}
	close(s.batteryStop)
	<-s.batteryDone
	s.batteryStop = nil
	s.batteryDone = nil
}

// batteryLoop periodically checks power state and auto-disables NoSleep
// when the system switches to battery or drops below the configured threshold.
// It also nudges the theme scheduler so dark-on-battery reacts within a few
// seconds instead of waiting for the regular theme schedule tick.
func (s *trayState) batteryLoop(stopCh <-chan struct{}, doneCh chan<- struct{}) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	defer close(doneCh)
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			select {
			case s.stateCh <- stateRequest{fn: func() string {
				s.refreshBatteryPolicy()
				return ""
			}}:
			case <-stopCh:
				return
			}
		}
	}
}

func (s *trayState) refreshBatteryPolicy() {
	ps := power.GetStatus()
	blocked := batteryPolicyBlocks(s.cfg, ps)
	if s.themeSched != nil {
		s.themeSched.CheckNow()
	}
	s.refreshTrayThemeIcon()
	if blocked == s.batteryBlocked {
		return
	}
	s.batteryBlocked = blocked
	mylog.Info("Battery policy changed: nosleep_blocked=%v", blocked)
	s.reconcileRuntime()

	s.updateIcon()
}

func batteryPolicyBlocks(cfg config.Config, status power.Status) bool {
	if !status.Valid || !status.Battery || status.ACLine {
		return false
	}
	return !cfg.NoSleepOnBattery ||
		(status.Percent >= 0 && status.Percent < cfg.NoSleepBatteryThreshold)
}

// ---- IPC handler ------------------------------------------------------

func (s *trayState) handleIPC(cmd string) string {
	return s.call(func() string { return s.handleIPCState(cmd) })
}

func (s *trayState) handleIPCState(cmd string) string {
	mylog.Info("IPC command received: %s", cmd)
	switch cmd {
	case "sleep":
		if err := s.executeAction(config.ActionSleep); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "hibernate":
		if err := s.executeAction(config.ActionHibernate); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "shutdown":
		if err := actions.Shutdown(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "lock":
		if err := actions.Lock(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"

	case "nosleep:on":
		s.setNoSleep(true, false)
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "nosleep:on:screen":
		s.setNoSleep(true, true)
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "nosleep:off":
		s.setNoSleep(false, s.cfg.KeepScreenOn)
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "nosleep:toggle":
		s.toggleNoSleep()
		return "ok"
	case "nosleep:status":
		return s.fmtNoSleepStatus()

	case "monitor:on":
		minutes := s.cfg.IdleTimeoutMinutes
		if minutes <= 0 {
			minutes = config.DefaultIdleTimeoutMinutes
		}
		s.setIdleTimeout(minutes)
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "monitor:off":
		s.setIdleTimeout(0)
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "monitor:status":
		return s.statusLine("status_monitor", s.monitorStatusText())

	case "status":
		return s.fmtStatus()

	case "ping":
		return "pong"
	case "open":
		if !systray.Post(s.showPopup) {
			return "err: tray UI is not ready"
		}
		return "ok"

	case "config:reload":
		mylog.Info("IPC config reload requested")
		if err := s.reloadConfig(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"

	default:
		return "err: unknown command: " + cmd
	}
}

func (s *trayState) reloadConfig() error {
	newCfg, err := config.Load()
	if err != nil {
		return err
	}
	s.cfg = newCfg
	if enabled, updated, err := autostart.EnsureCurrent(); err == nil {
		s.cfg.AutostartEnabled = enabled
		if updated {
			mylog.Info("Autostart entry updated to current executable path")
		}
	} else {
		mylog.Info("Autostart check failed: %v", err)
	}
	s.stopMonitor()
	s.stopHotkeys()
	s.stopProcessWatcher()
	s.stopThemeScheduler()
	nosleep.Disable()
	// Config compatibility: if both NoSleep and idle monitor are enabled,
	// resolve the conflict — NoSleep takes priority.
	if s.cfg.NoSleepEnabled && s.cfg.IdleTimeoutMinutes > 0 {
		s.cfg.IdleTimeoutMinutes = 0
	}
	s.lang = s.cfg.Language
	s.applyLanguage()
	s.batteryBlocked = false
	s.refreshBatteryPolicy()
	if s.cfg.HotkeysEnabled {
		s.startHotkeys()
	}
	s.syncProcessWatcher()
	if s.cfg.ThemeSwitchEnabled {
		s.startThemeScheduler()
	}
	s.refreshIPLocationInBackground()
	s.reconcileRuntime()
	s.updateIcon()
	s.rememberConfigModTime()
	systray.Post(popup.Hide)
	return nil
}

func (s *trayState) fmtNoSleepStatus() string {
	if nosleep.IsEnabled() {
		value := i18n.T(s.lang, "status_enabled")
		if nosleep.IsKeepingScreenOn() {
			value = i18n.T(s.lang, "status_enabled_keep_screen")
		}
		return s.statusLine("status_nosleep", value)
	}
	return s.statusLine("status_nosleep", i18n.T(s.lang, "status_disabled"))
}

func (s *trayState) fmtStatus() string {
	ns := i18n.T(s.lang, "status_disabled")
	if nosleep.IsEnabled() {
		ns = i18n.T(s.lang, "status_enabled")
		if nosleep.IsKeepingScreenOn() {
			ns = i18n.T(s.lang, "status_enabled_keep_screen")
		}
	}
	mon := s.monitorStatusText()
	ps := power.GetStatus()
	pow := i18n.T(s.lang, "status_unknown")
	if ps.Valid && ps.ACLine {
		pow = i18n.T(s.lang, "status_ac_power")
	} else if ps.Valid && ps.Battery && ps.Percent >= 0 {
		pow = fmt.Sprintf(i18n.T(s.lang, "status_battery"), ps.Percent)
	}

	idle := i18n.T(s.lang, "status_unknown")
	if d, err := monitor.IdleDuration(); err == nil {
		idle = i18n.FormatDuration(s.lang, d)
	}

	hk := i18n.T(s.lang, "status_disabled")
	if s.cfg.HotkeysEnabled {
		hk = i18n.T(s.lang, "status_enabled")
	}

	return strings.Join([]string{
		s.statusLine("status_tray", i18n.T(s.lang, "status_running")),
		s.statusLine("status_nosleep", ns),
		s.statusLine("status_monitor", mon),
		s.statusLine("status_power", pow),
		s.statusLine("status_idle_time", idle),
		s.statusLine("status_hotkeys", hk),
	}, "\n")
}

func (s *trayState) statusLine(labelKey, value string) string {
	return fmt.Sprintf(i18n.T(s.lang, "status_line"), i18n.T(s.lang, labelKey), value)
}

func (s *trayState) monitorStatusText() string {
	if s.mon != nil {
		if s.devtools.IdleMonitorEnabled() {
			return s.developerIdleMonitorStatus()
		}
		return fmt.Sprintf(
			i18n.T(s.lang, "status_monitor_active"),
			s.cfg.IdleTimeoutMinutes,
			i18n.T(s.lang, actionTranslationKey(s.cfg.IdleAction)),
		)
	}
	if s.idleSuspended() {
		return i18n.T(s.lang, "status_paused_by_nosleep")
	}
	return i18n.T(s.lang, "status_disabled")
}

// ---- helpers ----------------------------------------------------------

func (s *trayState) executeAction(a config.Action) error {
	if !actionAvailable(a, power.GetCapabilities()) {
		switch a {
		case config.ActionSleep:
			return errors.New(i18n.T(s.lang, "cli_error_sleep_unavailable"))
		case config.ActionHibernate:
			return errors.New(i18n.T(s.lang, "cli_error_hibernate_unavailable"))
		}
	}
	switch a {
	case config.ActionSleep:
		return actions.Sleep()
	case config.ActionHibernate:
		return actions.Hibernate()
	case config.ActionShutdown:
		return actions.Shutdown()
	case config.ActionLock:
		return actions.Lock()
	default:
		return fmt.Errorf("unsupported action %q", a)
	}
}

func actionAvailable(action config.Action, capabilities power.Capabilities) bool {
	switch action {
	case config.ActionSleep:
		return capabilities.SleepAvailable
	case config.ActionHibernate:
		return capabilities.HibernateAvailable
	default:
		return true
	}
}

func actionTranslationKey(a config.Action) string {
	return "menu_action_" + string(a)
}

// switchLanguage updates the active language, refreshes all menu text,
// and persists the choice.
func (s *trayState) switchLanguage(lang string) {
	s.lang = lang
	s.cfg.Language = lang
	s.applyLanguage()
	mylog.Info("Language switched: %s", lang)
	s.saveConfig()
}

func (s *trayState) applyLanguage() {
	T := func(key string) string { return i18n.T(s.lang, key) }
	systray.SetTitle(T("app_title"))
	if s.menuOpen != nil {
		s.menuOpen.SetTitle(T("menu_open_panel"))
	}
	if s.menuExit != nil {
		s.menuExit.SetTitle(T("menu_exit"))
	}
	s.updateIcon()
}

func (s *trayState) saveConfigErr() error {
	// The config watcher runs independently of the tray UI goroutine.  Mark the
	// whole atomic-replace window as self-authored so it cannot race the
	// mod-time snapshot and schedule reloadConfig (which hides the popup).
	s.selfConfigWrite.Store(true)
	defer s.selfConfigWrite.Store(false)
	if err := config.Save(s.cfg); err != nil {
		mylog.Info("Config save failed: %v", err)
		return err
	}
	s.rememberConfigModTime()
	if s.callbacks.OnConfigChanged != nil {
		s.callbacks.OnConfigChanged(s.cfg)
	}
	return nil
}

func (s *trayState) saveConfig() {
	if err := s.saveConfigErr(); err != nil {
		msg := fmt.Sprintf(i18n.T(s.lang, "msg_config_save_failed"), err.Error())
		dialog.Warn(i18n.T(s.lang, "app_title"), "", msg)
	}
}

func (s *trayState) showError(actionKey string, err error) {
	actionName := i18n.T(s.lang, actionKey)
	msg := fmt.Sprintf(i18n.T(s.lang, "msg_action_failed"), actionName, err.Error())
	dialog.Warn(i18n.T(s.lang, "app_title"), "", msg)
}

func (s *trayState) startThemeScheduler() {
	s.stopThemeScheduler()
	if s.cfg.ThemeLightTime == "" || s.cfg.ThemeDarkTime == "" {
		return
	}
	loc := themeswitch.LocationInfo{Latitude: s.cfg.ThemeLatitude, Longitude: s.cfg.ThemeLongitude, Source: themeswitch.LocationSourceConfigured}
	if s.cfg.ThemeMode == "sunrise" {
		loc = s.themeLocationInfo(false)
	}
	s.themeSched = themeswitch.NewScheduler(s.cfg.ThemeMode, s.cfg.ThemeLightTime, s.cfg.ThemeDarkTime, loc.Latitude, loc.Longitude, s.cfg.ThemeSkipFullscreen, s.cfg.ThemeDarkOnBattery)
	s.themeSched.Start()
	mylog.Info("Theme scheduler started: mode=%s light=%s dark=%s lat=%.4f lon=%.4f source=%s", s.cfg.ThemeMode, s.cfg.ThemeLightTime, s.cfg.ThemeDarkTime, loc.Latitude, loc.Longitude, loc.Source)
}

func (s *trayState) stopThemeScheduler() {
	if s.themeSched != nil {
		s.themeSched.Stop()
		s.themeSched = nil
	}
}

type popupSnapshot struct {
	state popup.State
	lang  string
}

func (s *trayState) showPopup() {
	s.openPopup(false)
}

func (s *trayState) refreshPopup() {
	s.openPopup(true)
}

func (s *trayState) openPopup(refresh bool) {
	result := make(chan popupSnapshot, 1)
	s.stateCh <- stateRequest{fn: func() string {
		result <- popupSnapshot{
			state: popup.State{
				NoSleepEnabled:          s.cfg.NoSleepEnabled,
				ProcessWatchEnabled:     s.cfg.ProcessWatchEnabled,
				IdleEnabled:             s.idleMonitorRequested(),
				IdlePaused:              s.idleSuspended(),
				IdleWarningEnabled:      s.cfg.IdleWarningSeconds > 0,
				IdleEnhancedMonitor:     s.cfg.IdleEnhancedMonitor,
				IdleTimeout:             s.cfg.IdleTimeoutMinutes,
				IdleWarningSeconds:      s.cfg.IdleWarningSeconds,
				IdleAction:              string(s.cfg.IdleAction),
				ProcessWatchList:        effectiveProcessWatchList(s.cfg),
				ProcessWatchActive:      s.processNoSleep,
				ThemeSwitchEnabled:      s.cfg.ThemeSwitchEnabled,
				DarkOnBattery:           s.cfg.ThemeDarkOnBattery,
				SkipFullscreen:          s.cfg.ThemeSkipFullscreen,
				IPLocationEnabled:       s.cfg.ThemeIPLocationEnabled,
				HotkeysEnabled:          s.cfg.HotkeysEnabled,
				AutostartEnabled:        s.cfg.AutostartEnabled,
				LoggingEnabled:          s.cfg.LoggingEnabled,
				IsChinese:               i18n.ResolveLanguage(s.lang) == "zh-CN",
				ThemeSchedule:           s.themeScheduleText(true),
				IPLocationLabel:         s.ipLocationLabel(),
				AppVersion:              version.Value,
				Owner:                   systray.WindowHandle(),
				DeveloperCapturePanel:   s.devtools.CapturePanel,
				DeveloperWarningPreview: s.devtools.WarningPreview,
			},
			lang: s.lang,
		}
		return ""
	}}
	snapshot := <-result
	onAction := func(action popup.Action, value int) {
		s.post(func() { s.handlePopupAction(action, value) })
	}
	langFn := func(key string) string { return i18n.T(snapshot.lang, key) }
	var err error
	if refresh {
		err = popup.Refresh(snapshot.state, onAction, langFn)
	} else {
		err = popup.Show(snapshot.state, onAction, langFn)
	}
	if err != nil {
		mylog.Info("Popup open failed: %v", err)
	}
}

func (s *trayState) handlePopupAction(action popup.Action, value int) {
	switch action {
	case popup.ActSleep, popup.ActHibernate, popup.ActShutdown, popup.ActLock:
		a := map[popup.Action]config.Action{
			popup.ActSleep:     config.ActionSleep,
			popup.ActHibernate: config.ActionHibernate,
			popup.ActShutdown:  config.ActionShutdown,
			popup.ActLock:      config.ActionLock,
		}[action]
		if err := s.executeAction(a); err != nil {
			s.showError(actionTranslationKey(a), err)
		}
	case popup.ActRestart:
		if err := actions.Restart(); err != nil {
			s.showError("menu_restart", err)
		}
	case popup.ActNoSleepToggle:
		s.toggleNoSleep()
	case popup.ActProcessWatchToggle:
		s.cfg.ProcessWatchEnabled = !s.cfg.ProcessWatchEnabled
		s.syncProcessWatcher()
		s.reconcileRuntime()
		s.updateIcon()
		s.saveConfig()
	case popup.ActIdleToggle:
		if s.cfg.IdleTimeoutMinutes > 0 {
			s.setIdleTimeout(0)
		} else {
			s.setIdleTimeout(config.DefaultIdleTimeoutMinutes)
		}
		s.saveConfig()
	case popup.ActIdleTimeout:
		if value > 0 && value <= 7*24*60 {
			s.setIdleTimeout(value)
			s.saveConfig()
		}
	case popup.ActIdleAction:
		if action, ok := config.IdleActionAt(value); ok {
			s.cfg.IdleAction = action
			s.reconcileRuntime()
			s.saveConfig()
		}
	case popup.ActIdleWarningToggle:
		if s.cfg.IdleWarningSeconds > 0 {
			s.cfg.IdleWarningSeconds = 0
		} else {
			s.cfg.IdleWarningSeconds = 30
		}
		s.reconcileRuntime()
		s.saveConfig()
	case popup.ActIdleEnhancedMonitorToggle:
		s.cfg.IdleEnhancedMonitor = !s.cfg.IdleEnhancedMonitor
		mylog.Info("Idle monitor enhanced mode toggled: enabled=%v", s.cfg.IdleEnhancedMonitor)
		s.reconcileRuntime()
		s.saveConfig()
	case popup.ActThemeToggle:
		s.cfg.ThemeSwitchEnabled = !s.cfg.ThemeSwitchEnabled
		if s.cfg.ThemeSwitchEnabled {
			s.startThemeScheduler()
		} else {
			s.stopThemeScheduler()
		}
		s.syncBatteryLoop()
		s.updateIcon()
		s.refreshPopupThemeSchedule()
		s.saveConfig()
	case popup.ActBatteryToggle:
		s.cfg.ThemeDarkOnBattery = !s.cfg.ThemeDarkOnBattery
		s.restartThemeScheduler()
		s.refreshPopupThemeSchedule()
		s.saveConfig()
	case popup.ActFullscreenToggle:
		s.cfg.ThemeSkipFullscreen = !s.cfg.ThemeSkipFullscreen
		s.restartThemeScheduler()
		s.refreshPopupThemeSchedule()
		s.saveConfig()
	case popup.ActIPLocationToggle:
		s.cfg.ThemeIPLocationEnabled = !s.cfg.ThemeIPLocationEnabled
		s.restartThemeScheduler()
		s.refreshIPLocationInBackground()
		s.updateIcon()
		s.refreshPopupThemeSchedule()
		s.saveConfig()
	case popup.ActSwitchTheme:
		mode := themeswitch.ModeDark
		if themeswitch.Current() == themeswitch.ModeDark {
			mode = themeswitch.ModeLight
		}
		if s.themeSched != nil {
			s.themeSched.HoldManualOverride(time.Now())
			mylog.Info("Manual theme override enabled until the next scheduled transition")
		}
		s.runThemeOperation("menu_theme_switch_now", func() error {
			return themeswitch.Switch(mode)
		}, nil)
	case popup.ActRepairTheme:
		s.runThemeOperation("menu_theme_repair", themeswitch.Refresh, nil)
	case popup.ActHotkeyToggle:
		s.cfg.HotkeysEnabled = !s.cfg.HotkeysEnabled
		if s.cfg.HotkeysEnabled {
			s.startHotkeys()
		} else {
			s.stopHotkeys()
		}
		s.saveConfig()
	case popup.ActAutostartToggle:
		enabled := !s.cfg.AutostartEnabled
		var err error
		if enabled {
			err = autostart.Enable()
		} else {
			err = autostart.Disable()
		}
		if err != nil {
			s.showError("menu_autostart", err)
			return
		}
		s.cfg.AutostartEnabled = enabled
		s.saveConfig()
	case popup.ActLoggingToggle:
		s.cfg.LoggingEnabled = !s.cfg.LoggingEnabled
		s.applyLogging()
		s.updateIcon()
		s.saveConfig()
	case popup.ActLanguage:
		previous := s.lang
		switch value {
		case 0:
			s.switchLanguage("en")
		case 1:
			s.switchLanguage("zh-CN")
		}
		if s.lang != previous {
			systray.Post(s.refreshPopup)
		}
	case popup.ActConfig:
		path, err := config.Path()
		if err != nil {
			mylog.Info("Config path lookup failed: %v", err)
			return
		}
		if err := openWithShell("notepad.exe", windows.EscapeArg(path)); err != nil {
			mylog.Info("Config editor launch failed: %v", err)
		}
	case popup.ActProjectHome:
		if err := openWithShell(projectHomeURL, ""); err != nil {
			mylog.Info("Project home launch failed: %v", err)
		}
	case popup.ActExit:
		popup.Destroy()
		systray.Quit()
	}
}

func (s *trayState) restartThemeScheduler() {
	if !s.cfg.ThemeSwitchEnabled {
		return
	}
	s.startThemeScheduler()
}

func (s *trayState) refreshPopupThemeSchedule() {
	text := s.themeScheduleText(true)
	systray.Post(func() {
		popup.UpdateThemeSchedule(text, s.ipLocationLabel())
	})
}

func (s *trayState) refreshIPLocationInBackground() {
	if !s.cfg.ThemeIPLocationEnabled || s.cfg.ThemeMode != "sunrise" || s.cfg.ThemeLatitude != 0 || s.cfg.ThemeLongitude != 0 {
		return
	}
	go func() {
		loc := themeswitch.AutoLocationInfo(true, true)
		if loc.Source != themeswitch.LocationSourceIP {
			return
		}
		s.post(func() {
			if !s.cfg.ThemeIPLocationEnabled || s.cfg.ThemeMode != "sunrise" || s.cfg.ThemeLatitude != 0 || s.cfg.ThemeLongitude != 0 {
				return
			}
			s.restartThemeScheduler()
			s.refreshPopupThemeSchedule()
			s.updateIcon()
		})
	}()
}

func (s *trayState) runThemeOperation(actionKey string, fn func() error, onSuccess func()) {
	go func() {
		if err := fn(); err != nil {
			s.post(func() { s.showError(actionKey, err) })
			return
		}
		s.post(func() {
			if onSuccess != nil {
				onSuccess()
			}
			s.refreshTrayThemeIcon()
			systray.Post(popup.RefreshTheme)
		})
	}()
}

func (s *trayState) applyLogging() {
	if s.cfg.LoggingEnabled {
		exePath, _ := os.Executable()
		mylog.Init(true, filepath.Dir(exePath))
		mylog.Info("Debug logging enabled from control panel")
		return
	}
	mylog.Info("Debug logging disabled from control panel")
	mylog.Close()
}

func (s *trayState) watchConfig() {
	cfgPath, err := config.Path()
	if err != nil {
		return
	}
	var lastMod time.Time
	if info, err := os.Stat(cfgPath); err == nil {
		lastMod = info.ModTime()
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		info, err := os.Stat(cfgPath)
		if err != nil {
			continue
		}
		modTime := info.ModTime()
		if modTime.After(lastMod) {
			lastMod = modTime
			if s.selfConfigWrite.Load() {
				continue
			}
			if modTime.UnixNano() == s.selfConfigMod.Load() {
				continue
			}
			s.post(func() {
				if err := s.reloadConfig(); err != nil {
					mylog.Info("config reload failed: %v", err)
					return
				}
				mylog.Info("config reloaded from disk")
			})
		}
	}
}

func (s *trayState) rememberConfigModTime() {
	path, err := config.Path()
	if err != nil {
		return
	}
	if info, err := os.Stat(path); err == nil {
		s.selfConfigMod.Store(info.ModTime().UnixNano())
	}
}
