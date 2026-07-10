// Package tray manages the Windows system-tray icon and context menu.
package tray

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/JeffioZ/idletrigger/assets"
	"github.com/JeffioZ/idletrigger/internal/actions"
	"github.com/JeffioZ/idletrigger/internal/autostart"
	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/dialog"
	"github.com/JeffioZ/idletrigger/internal/hotkey"
	"github.com/JeffioZ/idletrigger/internal/i18n"
	"github.com/JeffioZ/idletrigger/internal/ipc"
	mylog "github.com/JeffioZ/idletrigger/internal/log"
	"github.com/JeffioZ/idletrigger/internal/monitor"
	"github.com/JeffioZ/idletrigger/internal/nosleep"
	"github.com/JeffioZ/idletrigger/internal/notify"
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

var timeoutOptions = []struct {
	minutes int
	key     string
}{
	{5, "menu_timeout_5"},
	{10, "menu_timeout_10"},
	{30, "menu_timeout_30"},
	{60, "menu_timeout_60"},
	{120, "menu_timeout_120"},
}

var actionOptions = []struct {
	action config.Action
	key    string
}{
	{config.ActionSleep, "menu_action_sleep"},
	{config.ActionHibernate, "menu_action_hibernate"},
	{config.ActionShutdown, "menu_action_shutdown"},
	{config.ActionLock, "menu_action_lock"},
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

	// All menu items that display localised text — updated on language switch.

	// menu items (action items for capability checking)
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
	if nosleep.IsEnabled() {
		systray.SetIcon(assets.IconActive)
	} else if s.cfg.IdleTimeoutMinutes > 0 {
		systray.SetIcon(assets.IconMonitor)
	} else {
		systray.SetIcon(assets.IconDefault)
	}

	lines := []string{
		"IdleTrigger",
		s.tooltipStatus("status_nosleep", s.cfg.NoSleepEnabled),
	}
	if s.cfg.IdleTimeoutMinutes > 0 {
		act := i18n.T(s.lang, actionTranslationKey(s.cfg.IdleAction))
		lines[1] += " · " + s.statusLine("status_monitor", fmt.Sprintf(i18n.T(s.lang, "status_monitor_active"), s.cfg.IdleTimeoutMinutes, act))
	} else {
		lines[1] += " · " + s.tooltipStatus("status_monitor", false)
	}
	lines = append(lines, s.statusLine("menu_theme_switch", s.themeTooltipValue()))
	lines = append(lines, strings.Join([]string{
		s.tooltipStatus("menu_process_watch", s.cfg.ProcessWatchEnabled),
		s.tooltipStatus("status_hotkeys", s.cfg.HotkeysEnabled),
		s.tooltipStatus("menu_autostart", s.cfg.AutostartEnabled),
	}, " · "))
	systray.SetTooltip(compactTooltip(tooltipText(lines)))
}

func (s *trayState) tooltipStatus(labelKey string, enabled bool) string {
	value := i18n.T(s.lang, "status_disabled")
	if enabled {
		value = i18n.T(s.lang, "status_enabled")
	}
	return s.statusLine(labelKey, value)
}

func (s *trayState) themeTooltipValue() string {
	if !s.cfg.ThemeSwitchEnabled {
		return i18n.T(s.lang, "status_disabled")
	}
	parts := []string{i18n.T(s.lang, "status_enabled")}
	if s.cfg.ThemeMode == "sunrise" {
		parts = append(parts, i18n.T(s.lang, "menu_theme_sunrise"))
	} else {
		parts = append(parts, s.cfg.ThemeLightTime+"/"+s.cfg.ThemeDarkTime)
	}
	if schedule := s.themeScheduleText(); schedule != "" {
		parts = append(parts, schedule)
	}
	if s.cfg.ThemeDarkOnBattery {
		parts = append(parts, i18n.T(s.lang, "menu_theme_battery_dark"))
	}
	if s.cfg.ThemeSkipFullscreen {
		parts = append(parts, i18n.T(s.lang, "menu_theme_skip_fullscreen"))
	}
	return strings.Join(parts, " ")
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

func compactTooltip(value string) string {
	const maxRunes = 120
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(string(runes[:maxRunes-1])) + "…"
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

	s.mon = monitor.New(threshold, warnOffset,
		// onWarning — show balloon notification
		func() {
			actName := i18n.T(lang, actionTranslationKey(action))
			msg := fmt.Sprintf(i18n.T(lang, "msg_idle_warning"), actName, warningSeconds)
			title := i18n.T(lang, "app_title")
			if err := notify.Show(title, msg, warningSeconds); err != nil {
				mylog.Info("Idle warning notification failed: %v", err)
			}
		},
		// onTrigger
		func() {
			s.post(func() {
				if err := executeAction(action); err != nil {
					s.showError(actionTranslationKey(action), err)
				}
			})
		},
		0, // default poll interval
	)
	s.mon.Start()
}

func (s *trayState) stopMonitor() {
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
func (s *trayState) batteryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.post(func() { s.refreshBatteryPolicy() })
	}
}

func (s *trayState) refreshBatteryPolicy() {
	ps := power.GetStatus()
	blocked := batteryPolicyBlocks(s.cfg, ps)
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

func showAboutDialog(lang string) {
	ver := version.Value
	nl := string(rune(10))
	text := "IdleTrigger " + ver + nl + nl + i18n.T(lang, "about_body")
	dialog.Info(i18n.T(lang, "app_title"), "", text)
}

// registerLabel records a menu item so its text can be updated on language switch.

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
				IdleTimeout:         s.cfg.IdleTimeoutMinutes,
				IdleAction:          string(s.cfg.IdleAction),
				ThemeSwitchEnabled:  s.cfg.ThemeSwitchEnabled,
				SunriseMode:         s.cfg.ThemeMode == "sunrise",
				DarkOnBattery:       s.cfg.ThemeDarkOnBattery,
				SkipFullscreen:      s.cfg.ThemeSkipFullscreen,
				HotkeysEnabled:      s.cfg.HotkeysEnabled,
				AutostartEnabled:    s.cfg.AutostartEnabled,
				IsChinese:           s.lang == "zh-CN",
				ThemeSchedule:       s.themeScheduleText(),
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
		times := []int{5, 10, 30, 60, 120}
		if value >= 0 && value < len(times) {
			s.cfg.NoSleepEnabled = false
			s.cfg.IdleTimeoutMinutes = times[value]
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
	case popup.ActThemeToggle:
		s.cfg.ThemeSwitchEnabled = !s.cfg.ThemeSwitchEnabled
		if s.cfg.ThemeSwitchEnabled {
			s.startThemeScheduler()
		} else {
			s.stopThemeScheduler()
		}
		s.updateIcon()
		s.saveConfig()
	case popup.ActSunriseToggle:
		if s.cfg.ThemeMode == "sunrise" {
			s.cfg.ThemeMode = "fixed"
		} else {
			s.cfg.ThemeMode = "sunrise"
		}
		s.restartThemeScheduler()
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
	case popup.ActAbout:
		showAboutDialog(s.lang)
	case popup.ActExit:
		systray.Quit()
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
		}
	}()
}
