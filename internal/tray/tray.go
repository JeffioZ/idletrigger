// Package tray manages the Windows system-tray icon and context menu.
package tray

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/JeffioZ/idletrigger/assets"
	"github.com/JeffioZ/idletrigger/internal/actions"
	"github.com/JeffioZ/idletrigger/internal/autostart"
	"github.com/JeffioZ/idletrigger/internal/config"
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
)

type Callbacks struct {
	OnConfigChanged func(config.Config)
}

type trayState struct {
	cfg       config.Config
	lang      string
	callbacks Callbacks
	stateCh   chan stateRequest

	mon *monitor.Monitor

	hotkeyMgr      *hotkey.Manager
	procWatch      *processwatcher.Watcher
	themeSched     *themeswitch.Scheduler
	processNoSleep bool
	batteryBlocked bool
	trayThemeDark  bool
	menuOpen       *systray.MenuItem
	menuExit       *systray.MenuItem
}

type stateRequest struct {
	fn     func() string
	result chan string
}

// Run starts the system-tray loop. Blocks until Exit.
func Run(cfg config.Config, cbs Callbacks) {
	systray.SetErrorHandler(func(format string, args ...interface{}) {
		mylog.Info("Systray: "+format, args...)
	})

	s := &trayState{
		cfg:       cfg,
		lang:      cfg.Language,
		callbacks: cbs,
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
				popup.Destroy(); systray.Quit()
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
		if s.cfg.ProcessWatchEnabled && len(s.cfg.ProcessWatchList) > 0 {
			s.startProcessWatcher()
		}
		if s.cfg.ThemeSwitchEnabled {
			s.startThemeScheduler()
		}

		go s.stateLoop()
		go func() {
			if err := ipc.Server(s.handleIPC); err != nil {
				mylog.Info("IPC server stopped: %v", err)
			}
		}()
		go s.batteryLoop()

		systray.OnLeftClick = func() { s.showPopup() }
		// Pre-create the popup window so the first click opens instantly.
		// 预创建浮层窗口，首次点击即可瞬间展示。
		s.showPopup()
		popup.Hide()
		systray.OnPowerChange = func() {
			s.post(func() { s.refreshBatteryPolicy() })
		}
	}

	onExit := func() {
		s.call(func() string {
			s.stopMonitor()
			s.stopHotkeys()
			s.stopProcessWatcher()
			s.stopThemeScheduler()
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
		systray.SetIcon(assets.IconTrayLight)
	} else {
		systray.SetIcon(assets.IconTrayDark)
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
	} else if s.cfg.IdleTimeoutMinutes > 0 {
		lines = append(lines, s.statusLine("tooltip_idle", fmt.Sprintf("%d%s %s", s.cfg.IdleTimeoutMinutes, shortMinuteUnit(s.lang), i18n.T(s.lang, actionTranslationKey(s.cfg.IdleAction)))))
	} else {
		lines = append(lines, s.statusLine("tooltip_idle", shortStatus(s.lang, false)))
	}
	lines = append(lines, s.statusLine("tooltip_theme", s.themeTooltipValueShort()))
	if s.cfg.ProcessWatchEnabled {
		lines = append(lines, s.statusLine("tooltip_process", shortStatus(s.lang, s.processNoSleep)))
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
	if schedule := s.themeScheduleText(); schedule != "" {
		return i18n.T(s.lang, "status_short_on") + " " + compactThemeSchedule(schedule)
	}
	return i18n.T(s.lang, "status_short_on")
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

func (s *trayState) themeScheduleText() string {
	lat, lon := s.cfg.ThemeLatitude, s.cfg.ThemeLongitude
	if lat == 0 && lon == 0 {
		lat, lon = themeswitch.AutoLocation()
	}
	light, dark, ok := themeswitch.ScheduleTimes(s.cfg.ThemeMode, s.cfg.ThemeLightTime, s.cfg.ThemeDarkTime, lat, lon, time.Now())
	if !ok {
		return i18n.T(s.lang, "theme_schedule_unavailable")
	}
	return fmt.Sprintf(i18n.T(s.lang, "theme_schedule_format"), light, dark)
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
	if s.cfg.IdleTimeoutMinutes <= 0 {
		return
	}
	threshold := time.Duration(s.cfg.IdleTimeoutMinutes) * time.Minute
	warnOffset := time.Duration(s.cfg.IdleWarningSeconds) * time.Second
	mylog.Info("Idle monitor started: %d min -> %s", s.cfg.IdleTimeoutMinutes, string(s.cfg.IdleAction))
	action := s.cfg.IdleAction
	lang := s.lang
	warningSeconds := s.cfg.IdleWarningSeconds
	idlewarning.SetOnDismiss(func() {
		s.post(func() {
			if s.mon != nil && s.cfg.IdleTimeoutMinutes > 0 {
				s.startMonitor()
			}
		})
	})

	s.mon = monitor.New(threshold, warnOffset,
		// Show a non-activating in-app warning. Legacy notification-area
		// balloons are silently suppressed on many Windows 11 configurations.
		func() {
			actName := i18n.T(lang, actionTranslationKey(action))
			msg := fmt.Sprintf(i18n.T(lang, "msg_idle_warning"), actName, warningSeconds)
			title := i18n.T(lang, "app_title")
			mylog.Info("Idle warning displayed: action=%s seconds=%d", action, warningSeconds)
			idlewarning.Show(title, msg)
		},
		// Execute directly from the monitor goroutine. Waiting for the tray
		// state queue would make an overdue power action depend on UI work.
		func() {
			// The warning must be gone before a power action takes effect.
			systray.PostAndWait(idlewarning.Hide)
			idleFor, idleErr := monitor.IdleDuration()
			if idleErr != nil {
				mylog.Info("Idle monitor trigger reached, but idle duration lookup failed: %v", idleErr)
			} else {
				mylog.Info("Idle monitor trigger reached: idle=%s action=%s", idleFor.Round(time.Second), action)
			}
			if err := executeAction(action); err != nil {
				mylog.Info("Idle monitor action failed: action=%s error=%v", action, err)
				s.post(func() { s.showError(actionTranslationKey(action), err) })
				return
			}
			mylog.Info("Idle monitor action accepted: action=%s", action)
		},
		time.Second,
	)
	s.mon.SetOnActivity(func() { systray.Post(idlewarning.Hide) })
	s.mon.Start()
}

func (s *trayState) stopMonitor() {
	idlewarning.SetOnDismiss(nil)
	systray.Post(idlewarning.Hide)
	if s.mon != nil {
		s.mon.Stop()
		s.mon = nil
	}
}

func (s *trayState) reconcileRuntime() {
	wantsNoSleep := noSleepRequested(s.cfg, s.processNoSleep)
	if wantsNoSleep && !s.batteryBlocked {
		s.stopMonitor()
		nosleep.Enable(s.cfg.KeepScreenOn)
		return
	}

	nosleep.Disable()
	if s.cfg.IdleTimeoutMinutes > 0 {
		s.startMonitor()
	} else {
		s.stopMonitor()
	}
}

func noSleepRequested(cfg config.Config, processRequested bool) bool {
	return cfg.NoSleepEnabled || processRequested
}

// idleSuspended reports the intentional conflict resolution between process
// based keep-awake and the idle action. The saved idle setting is retained so
// it resumes automatically after the watched process exits.
func (s *trayState) idleSuspended() bool {
	return s.cfg.IdleTimeoutMinutes > 0 && s.processNoSleep && s.mon == nil
}

// ---- hotkeys ----------------------------------------------------------

func (s *trayState) startHotkeys() {
	s.stopHotkeys()
	mgr := hotkey.NewManager(hotkey.DefaultBindings(), hotkey.Callbacks{
		OnSleep: func() {
			s.post(func() {
				if err := actions.Sleep(); err != nil {
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
	if s.cfg.NoSleepEnabled {
		s.cfg.NoSleepEnabled = false
	} else {
		s.cfg.NoSleepEnabled = true
		s.cfg.IdleTimeoutMinutes = 0
	}
	s.reconcileRuntime()
	mylog.Info("NoSleep toggled: enabled=%v keep_screen_on=%v", nosleep.IsEnabled(), nosleep.IsKeepingScreenOn())

	s.updateIcon()
	s.saveConfig()
}

// applyCapabilities disables menu items for sleep/hibernate if the
// system does not support them, and adjusts the idle-action options.

// ---- process watcher --------------------------------------------------

func (s *trayState) startProcessWatcher() {
	s.stopProcessWatcher()
	if len(s.cfg.ProcessWatchList) == 0 {
		return
	}
	s.procWatch = processwatcher.New(s.cfg.ProcessWatchList,
		processwatcher.Callbacks{
			OnEnable: func() {
				s.post(func() {
					mylog.Info("Process watcher: watched app detected, requesting NoSleep")
					s.processNoSleep = true
					s.reconcileRuntime()

					s.updateIcon()
				})
			},
			OnDisable: func() {
				s.post(func() {
					mylog.Info("Process watcher: no watched apps running, releasing NoSleep")
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
	}
	s.processNoSleep = false
}

// ---- battery awareness ------------------------------------------------

// batteryLoop periodically checks power state and auto-disables NoSleep
// when the system switches to battery or drops below the configured threshold.
// It also nudges the theme scheduler so dark-on-battery reacts within a few
// seconds instead of waiting for the regular theme schedule tick.
func (s *trayState) batteryLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.post(func() { s.refreshBatteryPolicy() })
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
		(status.Percent > 0 && status.Percent < cfg.NoSleepBatteryThreshold)
}

// ---- IPC handler ------------------------------------------------------

func (s *trayState) handleIPC(cmd string) string {
	return s.call(func() string { return s.handleIPCState(cmd) })
}

func (s *trayState) handleIPCState(cmd string) string {
	mylog.Info("IPC command received: %s", cmd)
	switch cmd {
	case "sleep":
		if err := actions.Sleep(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "hibernate":
		if err := actions.Hibernate(); err != nil {
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
		s.cfg.NoSleepEnabled = true
		s.cfg.KeepScreenOn = false
		s.cfg.IdleTimeoutMinutes = 0
		s.reconcileRuntime()

		s.updateIcon()
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "nosleep:on:screen":
		s.cfg.NoSleepEnabled = true
		s.cfg.KeepScreenOn = true
		s.cfg.IdleTimeoutMinutes = 0
		s.reconcileRuntime()

		s.updateIcon()
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "nosleep:off":
		s.cfg.NoSleepEnabled = false
		s.reconcileRuntime()

		s.updateIcon()
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
		if s.cfg.IdleTimeoutMinutes <= 0 {
			s.cfg.IdleTimeoutMinutes = 30
		}
		s.cfg.NoSleepEnabled = false
		s.reconcileRuntime()

		s.updateIcon()
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "monitor:off":
		s.cfg.IdleTimeoutMinutes = 0
		s.reconcileRuntime()

		s.updateIcon()
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "monitor:status":
		if s.mon != nil {
			value := fmt.Sprintf(
				i18n.T(s.lang, "status_monitor_active"),
				s.cfg.IdleTimeoutMinutes,
				i18n.T(s.lang, actionTranslationKey(s.cfg.IdleAction)),
			)
			return s.statusLine("status_monitor", value)
		}
		return s.statusLine("status_monitor", i18n.T(s.lang, "status_disabled"))

	case "status":
		return s.fmtStatus()

	case "ping":
		return "pong"

	case "config:reload":
		mylog.Info("IPC config reload requested")
		newCfg, err := config.Load()
		if err != nil {
			return "err: " + err.Error()
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
		if s.cfg.ProcessWatchEnabled && len(s.cfg.ProcessWatchList) > 0 {
			s.startProcessWatcher()
		}
		if s.cfg.ThemeSwitchEnabled {
			s.startThemeScheduler()
		}
		s.reconcileRuntime()

		s.updateIcon()
		return "ok"

	default:
		return "err: unknown command: " + cmd
	}
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
	mon := i18n.T(s.lang, "status_disabled")
	if s.mon != nil {
		mon = fmt.Sprintf(
			i18n.T(s.lang, "status_monitor_active"),
			s.cfg.IdleTimeoutMinutes,
			i18n.T(s.lang, actionTranslationKey(s.cfg.IdleAction)),
		)
	}
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

// ---- helpers ----------------------------------------------------------

func executeAction(a config.Action) error {
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
	if err := config.Save(s.cfg); err != nil {
		mylog.Info("Config save failed: %v", err)
		return err
	}
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
	lat, lon := s.cfg.ThemeLatitude, s.cfg.ThemeLongitude
	if lat == 0 && lon == 0 {
		lat, lon = themeswitch.AutoLocation()
	}
	s.themeSched = themeswitch.NewScheduler(s.cfg.ThemeMode, s.cfg.ThemeLightTime, s.cfg.ThemeDarkTime, lat, lon, s.cfg.ThemeSkipFullscreen, s.cfg.ThemeDarkOnBattery)
	s.themeSched.Start()
	mylog.Info("Theme scheduler started: light=%s dark=%s", s.cfg.ThemeLightTime, s.cfg.ThemeDarkTime)
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
	result := make(chan popupSnapshot, 1)
	s.stateCh <- stateRequest{fn: func() string {
		result <- popupSnapshot{
			state: popup.State{
				NoSleepEnabled:      s.cfg.NoSleepEnabled,
				ProcessWatchEnabled: s.cfg.ProcessWatchEnabled,
				IdleEnabled:         s.cfg.IdleTimeoutMinutes > 0,
				IdlePaused:          s.idleSuspended(),
				IdleWarningEnabled:  s.cfg.IdleWarningSeconds > 0,
				IdleTimeout:         s.cfg.IdleTimeoutMinutes,
				IdleAction:          string(s.cfg.IdleAction),
				ThemeSwitchEnabled:  s.cfg.ThemeSwitchEnabled,
				DarkOnBattery:       s.cfg.ThemeDarkOnBattery,
				SkipFullscreen:      s.cfg.ThemeSkipFullscreen,
				HotkeysEnabled:      s.cfg.HotkeysEnabled,
				AutostartEnabled:    s.cfg.AutostartEnabled,
				LoggingEnabled:      s.cfg.LoggingEnabled,
				IsChinese:           i18n.ResolveLanguage(s.lang) == "zh-CN",
				ThemeSchedule:       s.themeScheduleText(),
				AppVersion:          version.Value,
				Owner:               systray.WindowHandle(),
			},
			lang: s.lang,
		}
		return ""
	}}
	snapshot := <-result
	if err := popup.Show(snapshot.state, func(action popup.Action, value int) {
		s.post(func() { s.handlePopupAction(action, value) })
	}, func(key string) string { return i18n.T(snapshot.lang, key) }); err != nil {
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
		if err := executeAction(a); err != nil {
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
		if s.cfg.ProcessWatchEnabled && len(s.cfg.ProcessWatchList) > 0 {
			s.startProcessWatcher()
		} else {
			s.stopProcessWatcher()
		}
		s.reconcileRuntime()
		s.updateIcon()
		s.saveConfig()
	case popup.ActIdleToggle:
		if s.cfg.IdleTimeoutMinutes > 0 {
			s.cfg.IdleTimeoutMinutes = 0
		} else {
			s.cfg.NoSleepEnabled = false
			s.cfg.IdleTimeoutMinutes = 30
		}
		s.reconcileRuntime()
		s.updateIcon()
		s.saveConfig()
	case popup.ActIdleTimeout:
		if value > 0 && value <= 7*24*60 {
			s.cfg.NoSleepEnabled = false
			s.cfg.IdleTimeoutMinutes = value
			s.reconcileRuntime()
			s.updateIcon()
			s.saveConfig()
		}
	case popup.ActIdleAction:
		actions := []config.Action{config.ActionSleep, config.ActionHibernate, config.ActionShutdown, config.ActionLock}
		if value >= 0 && value < len(actions) {
			s.cfg.IdleAction = actions[value]
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
	case popup.ActThemeToggle:
		s.cfg.ThemeSwitchEnabled = !s.cfg.ThemeSwitchEnabled
		if s.cfg.ThemeSwitchEnabled {
			s.startThemeScheduler()
		} else {
			s.stopThemeScheduler()
		}
		s.updateIcon()
		s.saveConfig()
	case popup.ActBatteryToggle:
		s.cfg.ThemeDarkOnBattery = !s.cfg.ThemeDarkOnBattery
		s.restartThemeScheduler()
		s.saveConfig()
	case popup.ActFullscreenToggle:
		s.cfg.ThemeSkipFullscreen = !s.cfg.ThemeSkipFullscreen
		s.restartThemeScheduler()
		s.saveConfig()
	case popup.ActSwitchTheme:
		mode := themeswitch.ModeDark
		if themeswitch.Current() == themeswitch.ModeDark {
			mode = themeswitch.ModeLight
		}
		s.runThemeOperation("menu_theme_switch_now", func() error {
			return themeswitch.Switch(mode)
		})
	case popup.ActRepairTheme:
		s.runThemeOperation("menu_theme_repair", themeswitch.Refresh)
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
		if value == 0 {
			s.switchLanguage("en")
		} else if value == 1 {
			s.switchLanguage("zh-CN")
		}
	case popup.ActConfig:
		path, err := config.Path()
		if err != nil {
			mylog.Info("Config path lookup failed: %v", err)
			return
		}
		if err := exec.Command("notepad.exe", path).Start(); err != nil {
			mylog.Info("Config editor launch failed: %v", err)
		}
	case popup.ActExit:
		popup.Destroy(); systray.Quit()
	}
}

func (s *trayState) restartThemeScheduler() {
	if !s.cfg.ThemeSwitchEnabled {
		return
	}
	s.startThemeScheduler()
}

func (s *trayState) runThemeOperation(actionKey string, fn func() error) {
	go func() {
		if err := fn(); err != nil {
			s.post(func() { s.showError(actionKey, err) })
			return
		}
		s.post(func() { s.refreshTrayThemeIcon() })
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
		if info.ModTime().After(lastMod) {
			lastMod = info.ModTime()
			newCfg, err := config.Load()
			if err != nil {
				mylog.Info("config reload failed: %v", err)
				continue
			}
			s.cfg = newCfg
			s.lang = s.cfg.Language
			s.reconcileRuntime()
			s.updateIcon()
			popup.Hide()
			mylog.Info("config reloaded from disk")
		}
	}
}

func (s *trayState) popupState() popup.State {
	return popup.State{
		NoSleepEnabled:      s.cfg.NoSleepEnabled,
		ProcessWatchEnabled: s.cfg.ProcessWatchEnabled,
		IdleEnabled:         s.cfg.IdleTimeoutMinutes > 0,
		IdleTimeout:         s.cfg.IdleTimeoutMinutes,
		IdleAction:          string(s.cfg.IdleAction),
		ThemeSwitchEnabled:  s.cfg.ThemeSwitchEnabled,
		DarkOnBattery:       s.cfg.ThemeDarkOnBattery,
		SkipFullscreen:      s.cfg.ThemeSkipFullscreen,
		HotkeysEnabled:      s.cfg.HotkeysEnabled,
		AutostartEnabled:    s.cfg.AutostartEnabled,
		IsChinese:           s.lang == "zh-CN",
		ThemeSchedule:       s.cfg.ThemeMode,
		LoggingEnabled:      s.cfg.LoggingEnabled,
	}
}
