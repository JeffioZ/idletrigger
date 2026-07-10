// Package tray manages the Windows system-tray icon and context menu.
package tray

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/getlantern/systray"

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
	"github.com/JeffioZ/idletrigger/internal/power"
	"github.com/JeffioZ/idletrigger/internal/processwatcher"
	"github.com/JeffioZ/idletrigger/internal/themeswitch"
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
	cfg   config.Config
	cfgMu sync.RWMutex
	lang  string

	mon   *monitor.Monitor
	monMu sync.Mutex

	hotkeyMgr  *hotkey.Manager
	procWatch  *processwatcher.Watcher
	themeSched *themeswitch.Scheduler

	// All menu items that display localised text — updated on language switch.
	labelItems []labelItem

	// menu items (action items for capability checking)
	mSleep          *systray.MenuItem
	mHibernate      *systray.MenuItem
	mShutdown       *systray.MenuItem
	mLock           *systray.MenuItem
	mNoSleep        *systray.MenuItem
	mIdleEnable     *systray.MenuItem
	mIdleTimeout    *systray.MenuItem
	mIdleAction     *systray.MenuItem
	mHotkeys        *systray.MenuItem
	mThemeSwitch    *systray.MenuItem
	mThemeLightAt   *systray.MenuItem
	mThemeDarkAt    *systray.MenuItem
	mThemeSwitchNow *systray.MenuItem
	mThemeRepair    *systray.MenuItem
	themeLightItems []*systray.MenuItem
	themeDarkItems  []*systray.MenuItem
	timeoutItems    []*systray.MenuItem
	actionItems     []*systray.MenuItem
}

type labelItem struct {
	item *systray.MenuItem
	key  string
}

// Run starts the system-tray loop. Blocks until Exit.
func Run(cfg config.Config, cbs Callbacks) {
	s := &trayState{cfg: cfg, lang: cfg.Language}

	go ipc.Server(s.handleIPC)

	// Start battery-awareness goroutine.
	go s.batteryLoop()

	onReady := func() {
		s.buildMenu()
		s.syncChecks()
		s.updateIcon()

		// Config compatibility: if both NoSleep and idle monitor are enabled,
		// resolve the conflict — NoSleep takes priority.
		// 配置兼容：若两者均启用，NoSleep 优先，自动禁用空闲监测。
		if s.cfg.NoSleepEnabled && s.cfg.IdleTimeoutMinutes > 0 {
			s.cfg.IdleTimeoutMinutes = 0
		}
		if s.cfg.IdleTimeoutMinutes > 0 {
			s.startMonitor()
		}
		if s.cfg.NoSleepEnabled {
			nosleep.Enable(true)
		}

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

		s.eventLoop()
	}

	onExit := func() {
		s.stopMonitor()
		s.stopHotkeys()
		s.stopProcessWatcher()
		s.stopThemeScheduler()
		nosleep.Disable()
	}

	systray.Run(onReady, onExit)
}

// ---- menu construction -------------------------------------------------

func (s *trayState) buildMenu() {
	T := func(key string) string { return i18n.T(s.lang, key) }

	systray.SetIcon(assets.IconDefault)
	systray.SetTitle(T("app_title"))
	systray.SetTooltip(T("tooltip_default"))

	s.mSleep = systray.AddMenuItem(T("menu_sleep"), "")
	s.registerLabel(s.mSleep, "menu_sleep")
	s.mHibernate = systray.AddMenuItem(T("menu_hibernate"), "")
	s.registerLabel(s.mHibernate, "menu_hibernate")
	s.mShutdown = systray.AddMenuItem(T("menu_shutdown"), "")
	s.registerLabel(s.mShutdown, "menu_shutdown")
	s.mLock = systray.AddMenuItem(T("menu_lock"), "")
	s.registerLabel(s.mLock, "menu_lock")
	// Disable actions the system does not support.
	s.applyCapabilities()

	systray.AddSeparator()

	s.mNoSleep = systray.AddMenuItemCheckbox(T("menu_nosleep"), "", s.cfg.NoSleepEnabled)
	s.registerLabel(s.mNoSleep, "menu_nosleep")

	systray.AddSeparator()

	idleOn := s.cfg.IdleTimeoutMinutes > 0
	s.mIdleEnable = systray.AddMenuItemCheckbox(T("menu_idle_enable"), "", idleOn)
	s.registerLabel(s.mIdleEnable, "menu_idle_enable")
	s.mIdleTimeout = s.mIdleEnable.AddSubMenuItem(T("menu_idle_timeout"), "")
	s.registerLabel(s.mIdleTimeout, "menu_idle_timeout")
	s.timeoutItems = make([]*systray.MenuItem, len(timeoutOptions))
	for i, opt := range timeoutOptions {
		checked := s.cfg.IdleTimeoutMinutes == opt.minutes
		s.timeoutItems[i] = s.mIdleTimeout.AddSubMenuItemCheckbox(T(opt.key), "", checked)
		s.registerLabel(s.timeoutItems[i], opt.key)
	}
	s.mIdleAction = s.mIdleEnable.AddSubMenuItem(T("menu_idle_action"), "")
	s.registerLabel(s.mIdleAction, "menu_idle_action")
	s.actionItems = make([]*systray.MenuItem, len(actionOptions))
	for i, opt := range actionOptions {
		checked := s.cfg.IdleAction == opt.action
		s.actionItems[i] = s.mIdleAction.AddSubMenuItemCheckbox(T(opt.key), "", checked)
		s.registerLabel(s.actionItems[i], opt.key)
	}

	// Auto Theme Switch submenu
	systray.AddSeparator()
	s.mThemeSwitch = systray.AddMenuItemCheckbox(T("menu_theme_switch"), "", s.cfg.ThemeSwitchEnabled)
	s.registerLabel(s.mThemeSwitch, "menu_theme_switch")

	// Light time submenu
	s.mThemeLightAt = s.mThemeSwitch.AddSubMenuItem(T("menu_theme_light_time"), "")
	s.registerLabel(s.mThemeLightAt, "menu_theme_light_time")
	s.themeLightItems = make([]*systray.MenuItem, 3)
	lightKeys := []string{"theme_time_0600", "theme_time_0700", "theme_time_0800"}
	lightVals := []string{"06:00", "07:00", "08:00"}
	for i := 0; i < 3; i++ {
		checked := s.cfg.ThemeLightTime == lightVals[i]
		s.themeLightItems[i] = s.mThemeLightAt.AddSubMenuItemCheckbox(T(lightKeys[i]), "", checked)
		s.registerLabel(s.themeLightItems[i], lightKeys[i])
	}

	// Dark time submenu
	s.mThemeDarkAt = s.mThemeSwitch.AddSubMenuItem(T("menu_theme_dark_time"), "")
	s.registerLabel(s.mThemeDarkAt, "menu_theme_dark_time")
	s.themeDarkItems = make([]*systray.MenuItem, 4)
	darkKeys := []string{"theme_time_1800", "theme_time_1900", "theme_time_2000", "theme_time_2100"}
	darkVals := []string{"18:00", "19:00", "20:00", "21:00"}
	for i := 0; i < 4; i++ {
		checked := s.cfg.ThemeDarkTime == darkVals[i]
		s.themeDarkItems[i] = s.mThemeDarkAt.AddSubMenuItemCheckbox(T(darkKeys[i]), "", checked)
		s.registerLabel(s.themeDarkItems[i], darkKeys[i])
	}

	// Manual switch + repair
	s.mThemeSwitchNow = s.mThemeSwitch.AddSubMenuItem(T("menu_theme_switch_now"), "")
	s.registerLabel(s.mThemeSwitchNow, "menu_theme_switch_now")
	s.mThemeRepair = s.mThemeSwitch.AddSubMenuItem(T("menu_theme_repair"), "")
	s.registerLabel(s.mThemeRepair, "menu_theme_repair")

	systray.AddSeparator()
	s.mHotkeys = systray.AddMenuItemCheckbox(T("menu_hotkeys"), "", s.cfg.HotkeysEnabled)
	s.registerLabel(s.mHotkeys, "menu_hotkeys")
	mAutostart := systray.AddMenuItemCheckbox(T("menu_autostart"), "", s.cfg.AutostartEnabled)
	s.registerLabel(mAutostart, "menu_autostart")

	// Language submenu
	mLang := systray.AddMenuItem(T("menu_language"), "")
	s.registerLabel(mLang, "menu_language")
	mLangEn := mLang.AddSubMenuItemCheckbox(T("menu_lang_en"), "", s.cfg.Language != "zh-CN")
	s.registerLabel(mLangEn, "menu_lang_en")
	mLangZh := mLang.AddSubMenuItemCheckbox(T("menu_lang_zh"), "", s.cfg.Language == "zh-CN")
	s.registerLabel(mLangZh, "menu_lang_zh")
	mOpenCfg := systray.AddMenuItem(T("menu_open_config"), "")
	s.registerLabel(mOpenCfg, "menu_open_config")
	mAbout := systray.AddMenuItem(T("menu_about"), "")
	s.registerLabel(mAbout, "menu_about")

	systray.AddSeparator()
	mExit := systray.AddMenuItem(T("menu_exit"), "")
	s.registerLabel(mExit, "menu_exit")

	// ---- main event loop goroutine ---------------------------------------
	go func() {
		for {
			select {
			case <-s.mSleep.ClickedCh:
				if err := actions.Sleep(); err != nil {
					s.showError("menu_sleep", err)
				}
			case <-s.mHibernate.ClickedCh:
				if err := actions.Hibernate(); err != nil {
					s.showError("menu_hibernate", err)
				}
			case <-s.mShutdown.ClickedCh:
				if err := actions.Shutdown(); err != nil {
					s.showError("menu_shutdown", err)
				}
			case <-s.mLock.ClickedCh:
				if err := actions.Lock(); err != nil {
					s.showError("menu_lock", err)
				}

			case <-s.mNoSleep.ClickedCh:
				if nosleep.IsEnabled() {
					nosleep.Disable()
					s.cfg.NoSleepEnabled = false
				} else {
					nosleep.Enable(true)
					s.cfg.NoSleepEnabled = true
					// NoSleep and idle monitor are mutually exclusive — auto-disable idle monitor.
					s.cfg.IdleTimeoutMinutes = 0
					s.stopMonitor()
				}
				s.syncChecks()
				config.Save(s.cfg)

			case <-s.mIdleEnable.ClickedCh:
				if s.mIdleEnable.Checked() {
					s.mIdleEnable.Uncheck()
					s.cfg.IdleTimeoutMinutes = 0
					s.stopMonitor()
				} else {
					if nosleep.IsEnabled() {
						nosleep.Disable()
						s.cfg.NoSleepEnabled = false
					}
					s.mIdleEnable.Check()
					if s.cfg.IdleTimeoutMinutes <= 0 {
						s.cfg.IdleTimeoutMinutes = 30
					}
					s.startMonitor()
				}
				s.syncChecks()
				s.updateIcon()
				config.Save(s.cfg)

			case <-mAutostart.ClickedCh:
				if mAutostart.Checked() {
					mAutostart.Uncheck()
					autostart.Disable()
					s.cfg.AutostartEnabled = false
				} else {
					mAutostart.Check()
					autostart.Enable()
					s.cfg.AutostartEnabled = true
				}
				config.Save(s.cfg)

			case <-s.mThemeSwitchNow.ClickedCh:
				cur := themeswitch.Current()
				if cur == themeswitch.ModeDark {
					themeswitch.Switch(themeswitch.ModeLight)
				} else {
					themeswitch.Switch(themeswitch.ModeDark)
				}

			case <-s.mThemeRepair.ClickedCh:
				themeswitch.Switch(themeswitch.Current())

			case <-s.mThemeSwitch.ClickedCh:
				if s.mThemeSwitch.Checked() {
					s.mThemeSwitch.Uncheck()
					s.cfg.ThemeSwitchEnabled = false
					s.stopThemeScheduler()
				} else {
					s.mThemeSwitch.Check()
					s.cfg.ThemeSwitchEnabled = true
					s.startThemeScheduler()
				}
				config.Save(s.cfg)

			case <-s.mHotkeys.ClickedCh:
				if s.mHotkeys.Checked() {
					s.mHotkeys.Uncheck()
					s.cfg.HotkeysEnabled = false
					s.stopHotkeys()
				} else {
					s.mHotkeys.Check()
					s.cfg.HotkeysEnabled = true
					s.startHotkeys()
				}
				config.Save(s.cfg)

			case <-mLangEn.ClickedCh:
				s.switchLanguage("en")
				mLangEn.Check()
				mLangZh.Uncheck()

			case <-mLangZh.ClickedCh:
				s.switchLanguage("zh-CN")
				mLangZh.Check()
				mLangEn.Uncheck()

			case <-mOpenCfg.ClickedCh:
				p, _ := config.Path()
				exec.Command("notepad.exe", p).Start()

			case <-mAbout.ClickedCh:
				showAboutDialog(s.lang)

			case <-mExit.ClickedCh:
				s.stopMonitor()
				s.stopHotkeys()
				s.stopProcessWatcher()
				nosleep.Disable()
				systray.Quit()
				return
			}
		}
	}()

	s.wireSubmenus()
	s.wireThemeSubmenus()
}

func (s *trayState) eventLoop() {
	// Block forever — the goroutines above handle events.
	select {}
}

func (s *trayState) wireSubmenus() {
	for i, item := range s.timeoutItems {
		idx := i
		it := item
		go func() {
			for range it.ClickedCh {
				s.cfg.IdleTimeoutMinutes = timeoutOptions[idx].minutes
				s.updateTimeoutChecks()
				s.startMonitor()
				s.updateIcon()
				config.Save(s.cfg)
			}
		}()
	}
	for i, item := range s.actionItems {
		idx := i
		it := item
		go func() {
			for range it.ClickedCh {
				s.cfg.IdleAction = actionOptions[idx].action
				s.updateActionChecks()
				s.startMonitor()
				config.Save(s.cfg)
			}
		}()
	}
}

// ---- state sync -------------------------------------------------------

func (s *trayState) syncChecks() {
	// NoSleep and idle monitor are mutually exclusive — when one is active,
	// the other is visually disabled (grayed out) in the menu.
	// NoSleep 与空闲监测互斥，一方启用时另一方菜单变灰禁用。
	if s.cfg.NoSleepEnabled {
		s.mNoSleep.Check()
		s.mIdleEnable.Disable()
	} else {
		s.mNoSleep.Uncheck()
		s.mIdleEnable.Enable()
	}
	// Config compatibility: if both NoSleep and idle monitor are enabled,
	// resolve the conflict — NoSleep takes priority.
	// 配置兼容：若两者均启用，NoSleep 优先，自动禁用空闲监测。
	if s.cfg.NoSleepEnabled && s.cfg.IdleTimeoutMinutes > 0 {
		s.cfg.IdleTimeoutMinutes = 0
	}
	if s.cfg.IdleTimeoutMinutes > 0 {
		s.mIdleEnable.Check()
		s.mNoSleep.Disable()
	} else {
		s.mIdleEnable.Uncheck()
		s.mNoSleep.Enable()
	}
	if s.cfg.HotkeysEnabled {
		s.mHotkeys.Check()
	} else {
		s.mHotkeys.Uncheck()
	}
	if s.cfg.ThemeSwitchEnabled {
		s.mThemeSwitch.Check()
	} else {
		s.mThemeSwitch.Uncheck()
	}
	s.updateTimeoutChecks()
	s.updateActionChecks()
}

func (s *trayState) updateTimeoutChecks() {
	for i, item := range s.timeoutItems {
		if s.cfg.IdleTimeoutMinutes == timeoutOptions[i].minutes {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
}

func (s *trayState) updateActionChecks() {
	for i, item := range s.actionItems {
		if s.cfg.IdleAction == actionOptions[i].action {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
}

// ---- icon state -------------------------------------------------------

func (s *trayState) updateIcon() {
	if nosleep.IsEnabled() {
		systray.SetIcon(assets.IconActive)
		systray.SetTooltip(i18n.T(s.lang, "tooltip_nosleep"))
		// Config compatibility: if both NoSleep and idle monitor are enabled,
		// resolve the conflict — NoSleep takes priority.
		// 配置兼容：若两者均启用，NoSleep 优先，自动禁用空闲监测。
		if s.cfg.NoSleepEnabled && s.cfg.IdleTimeoutMinutes > 0 {
			s.cfg.IdleTimeoutMinutes = 0
		}
	} else if s.cfg.IdleTimeoutMinutes > 0 {
		systray.SetIcon(assets.IconMonitor)
		actName := string(s.cfg.IdleAction)
		systray.SetTooltip(fmt.Sprintf(i18n.T(s.lang, "tooltip_monitor"),
			s.cfg.IdleTimeoutMinutes, actName))
	} else {
		systray.SetIcon(assets.IconDefault)
		systray.SetTooltip(i18n.T(s.lang, "tooltip_default"))
	}
}

// ---- idle monitor -----------------------------------------------------

func (s *trayState) startMonitor() {
	s.stopMonitor()
	s.cfgMu.RLock()
	if s.cfg.IdleTimeoutMinutes <= 0 {
		s.cfgMu.RUnlock()
		return
	}
	threshold := time.Duration(s.cfg.IdleTimeoutMinutes) * time.Minute
	warnOffset := time.Duration(s.cfg.IdleWarningSeconds) * time.Second
	mylog.Info("Idle monitor started: %d min -> %s", s.cfg.IdleTimeoutMinutes, string(s.cfg.IdleAction))
	action := s.cfg.IdleAction
	lang := s.lang

	s.monMu.Lock()
	s.mon = monitor.New(threshold, warnOffset,
		// onWarning — show balloon notification
		func() {
			actName := string(action)
			msg := fmt.Sprintf(i18n.T(lang, "msg_idle_warning"), actName, s.cfg.IdleWarningSeconds)
			title := i18n.T(lang, "app_title")
			// Use a short timeout so it disappears if user comes back.
			notify.Show(0, title, msg, s.cfg.IdleWarningSeconds)
		},
		// onTrigger
		func() {
			executeAction(action, lang)
		},
		0, // default poll interval
	)
	s.mon.Start()
	s.monMu.Unlock()
	s.cfgMu.RUnlock()
}

func (s *trayState) stopMonitor() {
	s.monMu.Lock()
	defer s.monMu.Unlock()
	if s.mon != nil {
		s.mon.Stop()
		s.mon = nil
	}
}

// ---- hotkeys ----------------------------------------------------------

func (s *trayState) startHotkeys() {
	s.stopHotkeys()
	mgr := hotkey.NewManager(hotkey.DefaultBindings(), hotkey.Callbacks{
		OnSleep:         func() { actions.Sleep() },
		OnLock:          func() { actions.Lock() },
		OnToggleNoSleep: s.toggleNoSleep,
	})
	s.hotkeyMgr = mgr
	go func() {
		failed := mgr.Start()
		if len(failed) > 0 {
			s.showHotkeyConflict(failed)
		}
	}()
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
	if nosleep.IsEnabled() {
		nosleep.Disable()
		s.cfg.NoSleepEnabled = false
	} else {
		nosleep.Enable(true)
		s.cfg.NoSleepEnabled = true
		mylog.Info("NoSleep toggled: enabled=%v screen=%v", nosleep.IsEnabled(), nosleep.IsKeepingScreenOn())
	}
	s.syncChecks()
	s.updateIcon()
	config.Save(s.cfg)
}

// applyCapabilities disables menu items for sleep/hibernate if the
// system does not support them, and adjusts the idle-action options.
func (s *trayState) applyCapabilities() {
	caps := power.GetCapabilities()
	if !caps.SleepAvailable {
		s.mSleep.Disable()
	}
	if !caps.HibernateAvailable {
		s.mHibernate.Disable()
	}
	// Shutdown and Lock are always available.

	// If the configured idle action is unavailable, fall back to Lock.
	if s.cfg.IdleAction == config.ActionSleep && !caps.SleepAvailable {
		s.cfg.IdleAction = config.ActionLock
	}
	if s.cfg.IdleAction == config.ActionHibernate && !caps.HibernateAvailable {
		s.cfg.IdleAction = config.ActionLock
	}

	// Hide idle-action submenu items that are unavailable.
	for i, opt := range actionOptions {
		if opt.action == config.ActionSleep && !caps.SleepAvailable {
			s.actionItems[i].Hide()
		}
		if opt.action == config.ActionHibernate && !caps.HibernateAvailable {
			s.actionItems[i].Hide()
		}
	}
}

// ---- process watcher --------------------------------------------------

func (s *trayState) startProcessWatcher() {
	s.stopProcessWatcher()
	if len(s.cfg.ProcessWatchList) == 0 {
		return
	}
	s.procWatch = processwatcher.New(s.cfg.ProcessWatchList,
		processwatcher.Callbacks{
			OnEnable: func() {
				mylog.Info("ProcessWatch: watched app detected, enabling NoSleep")
				if !nosleep.IsEnabled() {
					nosleep.Enable(true)
					s.cfg.NoSleepEnabled = true
					s.cfg.IdleTimeoutMinutes = 0
					s.stopMonitor()
					s.syncChecks()
					s.updateIcon()
				}
			},
			OnDisable: func() {
				mylog.Info("ProcessWatch: no watched apps running, disabling NoSleep")
				if nosleep.IsEnabled() {
					nosleep.Disable()
					s.cfg.NoSleepEnabled = false
					s.syncChecks()
					s.updateIcon()
				}
			},
		}, 0)
	s.procWatch.Start()
}

func (s *trayState) stopProcessWatcher() {
	if s.procWatch != nil {
		s.procWatch.Stop()
		s.procWatch = nil
	}
}

// ---- battery awareness ------------------------------------------------

// batteryLoop periodically checks power state and auto-disables NoSleep
// when the system switches to battery or drops below the configured threshold.
// 定期检查电源状态，在切换到电池供电或低于阈值时自动禁用 NoSleep。
func (s *trayState) batteryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.cfgMu.RLock()
		onBattery := !s.cfg.NoSleepOnBattery && power.OnBattery() && nosleep.IsEnabled()
		ps := power.GetStatus()
		lowBattery := ps.Battery && ps.Percent > 0 && ps.Percent < s.cfg.NoSleepBatteryThreshold && nosleep.IsEnabled()
		s.cfgMu.RUnlock()

		if onBattery || lowBattery {
			mylog.Info("Battery: auto-disabling NoSleep (onBattery=%v lowBattery=%v)", onBattery, lowBattery)
			nosleep.Disable()
			s.cfg.NoSleepEnabled = false
			s.syncChecks()
			s.updateIcon()
		}
	}
}

// ---- IPC handler ------------------------------------------------------

func (s *trayState) handleIPC(cmd string) string {
	mylog.Info("IPC command received: %s", cmd)
	switch cmd {
	case "sleep":
		actions.Sleep()
		return "ok"
	case "hibernate":
		actions.Hibernate()
		return "ok"
	case "shutdown":
		actions.Shutdown()
		return "ok"
	case "lock":
		actions.Lock()
		return "ok"

	case "nosleep:on":
		nosleep.Enable(false)
		s.cfg.NoSleepEnabled = true
		s.cfg.KeepScreenOn = false
		s.cfg.IdleTimeoutMinutes = 0
		s.stopMonitor()
		s.syncChecks()
		s.updateIcon()
		config.Save(s.cfg)
		return "ok"
	case "nosleep:on:screen":
		nosleep.Enable(true)
		s.cfg.NoSleepEnabled = true
		s.cfg.KeepScreenOn = true
		s.cfg.IdleTimeoutMinutes = 0
		s.stopMonitor()
		s.syncChecks()
		s.updateIcon()
		config.Save(s.cfg)
		return "ok"
	case "nosleep:off":
		nosleep.Disable()
		s.cfg.NoSleepEnabled = false
		s.syncChecks()
		s.updateIcon()
		config.Save(s.cfg)
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
		if nosleep.IsEnabled() {
			nosleep.Disable()
			s.cfg.NoSleepEnabled = false
		}
		s.startMonitor()
		s.syncChecks()
		s.updateIcon()
		config.Save(s.cfg)
		return "ok"
	case "monitor:off":
		s.cfg.IdleTimeoutMinutes = 0
		s.stopMonitor()
		s.syncChecks()
		s.updateIcon()
		config.Save(s.cfg)
		return "ok"
	case "monitor:status":
		if s.mon != nil {
			return fmt.Sprintf("monitor: %d min → %s", s.cfg.IdleTimeoutMinutes, string(s.cfg.IdleAction))
		}
		return "monitor: disabled"

	case "status":
		return s.fmtStatus()

	case "ping":
		return "pong"

	case "config:reload":
		mylog.Info("IPC: config reload requested")
		newCfg, err := config.Load()
		if err != nil {
			return "err: " + err.Error()
		}
		s.cfg = newCfg
		s.stopMonitor()
		s.stopHotkeys()
		s.stopProcessWatcher()
		nosleep.Disable()
		// Config compatibility: if both NoSleep and idle monitor are enabled,
		// resolve the conflict — NoSleep takes priority.
		// 配置兼容：若两者均启用，NoSleep 优先，自动禁用空闲监测。
		if s.cfg.NoSleepEnabled && s.cfg.IdleTimeoutMinutes > 0 {
			s.cfg.IdleTimeoutMinutes = 0
		}
		if s.cfg.IdleTimeoutMinutes > 0 {
			s.startMonitor()
		}
		if s.cfg.NoSleepEnabled {
			nosleep.Enable(true)
		}
		if s.cfg.HotkeysEnabled {
			s.startHotkeys()
		}
		if s.cfg.ProcessWatchEnabled && len(s.cfg.ProcessWatchList) > 0 {
			s.startProcessWatcher()
		}
		s.syncChecks()
		s.updateIcon()
		return "ok"

	default:
		return "err: unknown command: " + cmd
	}
}

func (s *trayState) fmtNoSleepStatus() string {
	if nosleep.IsEnabled() {
		scr := ""
		if nosleep.IsKeepingScreenOn() {
			scr = " (keep screen on)"
		}
		return "nosleep: enabled" + scr
	}
	return "nosleep: disabled"
}

func (s *trayState) fmtStatus() string {
	ns := "disabled"
	if nosleep.IsEnabled() {
		ns = "enabled"
		if nosleep.IsKeepingScreenOn() {
			ns += " (keep screen on)"
		}
	}
	mon := "disabled"
	if s.mon != nil {
		mon = fmt.Sprintf("%d min → %s", s.cfg.IdleTimeoutMinutes, string(s.cfg.IdleAction))
	}
	ps := power.GetStatus()
	pow := "AC"
	if ps.Battery {
		pow = fmt.Sprintf("Battery %d%%", ps.Percent)
	}

	idle := "unknown"
	if d, err := monitor.IdleDuration(); err == nil {
		idle = d.Round(time.Second).String()
	}

	hk := "disabled"
	if s.cfg.HotkeysEnabled {
		hk = "enabled"
	}

	return fmt.Sprintf(
		"NoSleep:      %s\nIdle Monitor: %s\nPower:        %s\nIdle time:    %s\nHotkeys:      %s",
		ns, mon, pow, idle, hk,
	)
}

// ---- helpers ----------------------------------------------------------

func executeAction(a config.Action, lang string) {
	switch a {
	case config.ActionSleep:
		actions.Sleep()
	case config.ActionHibernate:
		actions.Hibernate()
	case config.ActionShutdown:
		actions.Shutdown()
	case config.ActionLock:
		actions.Lock()
	}
}

func showAboutDialog(lang string) {
	dialog.Info(
		i18n.T(lang, "app_title"),
		i18n.T(lang, "app_title"),
		i18n.T(lang, "about_text"),
	)
}

// registerLabel records a menu item so its text can be updated on language switch.
func (s *trayState) registerLabel(item *systray.MenuItem, key string) {
	s.labelItems = append(s.labelItems, labelItem{item, key})
}

// switchLanguage updates the active language, refreshes all menu text,
// and persists the choice.
func (s *trayState) switchLanguage(lang string) {
	s.lang = lang
	s.cfg.Language = lang
	T := func(key string) string { return i18n.T(s.lang, key) }
	for _, li := range s.labelItems {
		li.item.SetTitle(T(li.key))
		mylog.Info("Language switched: %s", lang)
	}
	systray.SetTitle(T("app_title"))
	s.updateIcon()
	config.Save(s.cfg)
}

func (s *trayState) showError(actionKey string, err error) {
	actionName := i18n.T(s.lang, actionKey)
	msg := fmt.Sprintf(i18n.T(s.lang, "msg_action_failed"), actionName+": "+err.Error())
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

func (s *trayState) wireThemeSubmenus() {
	// Light time radio
	lightVals := []string{"06:00", "07:00", "08:00"}
	for i, item := range s.themeLightItems {
		idx := i
		it := item
		go func() {
			for range it.ClickedCh {
				s.cfg.ThemeLightTime = lightVals[idx]
				for j, ti := range s.themeLightItems {
					if j == idx {
						ti.Check()
					} else {
						ti.Uncheck()
					}
				}
				s.stopThemeScheduler()
				if s.cfg.ThemeSwitchEnabled {
					s.startThemeScheduler()
				}
				config.Save(s.cfg)
			}
		}()
	}
	// Dark time radio
	darkVals := []string{"18:00", "19:00", "20:00", "21:00"}
	for i, item := range s.themeDarkItems {
		idx := i
		it := item
		go func() {
			for range it.ClickedCh {
				s.cfg.ThemeDarkTime = darkVals[idx]
				for j, ti := range s.themeDarkItems {
					if j == idx {
						ti.Check()
					} else {
						ti.Uncheck()
					}
				}
				s.stopThemeScheduler()
				if s.cfg.ThemeSwitchEnabled {
					s.startThemeScheduler()
				}
				config.Save(s.cfg)
			}
		}()
	}
}
