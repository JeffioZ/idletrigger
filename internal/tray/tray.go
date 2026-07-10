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
	labelItems []labelItem

	// menu items (action items for capability checking)
	mSleep          *systray.MenuItem
	mHibernate      *systray.MenuItem
	mShutdown       *systray.MenuItem
	mLock           *systray.MenuItem
	mNoSleep        *systray.MenuItem
	mNoSleepEnable  *systray.MenuItem
	mProcessWatch  *systray.MenuItem
	mIdleEnable     *systray.MenuItem
	mIdleTimeout    *systray.MenuItem
	mIdleAction     *systray.MenuItem
	mHotkeys        *systray.MenuItem
	mAutostart      *systray.MenuItem
	mThemeSwitch    *systray.MenuItem
	mThemeEnable    *systray.MenuItem
	mThemeLightAt   *systray.MenuItem
	mThemeDarkAt    *systray.MenuItem
	mThemeSwitchNow      *systray.MenuItem
	mThemeRepair         *systray.MenuItem
	mThemeSunrise        *systray.MenuItem
	mThemeBatteryDark    *systray.MenuItem
	mThemeSkipFullscreen *systray.MenuItem
	themeLightItems []*systray.MenuItem
	themeDarkItems  []*systray.MenuItem
	timeoutItems    []*systray.MenuItem
	actionItems     []*systray.MenuItem
}

type stateRequest struct {
	fn     func() string
	result chan string
}

type labelItem struct {
	item *systray.MenuItem
	key  string
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
		if enabled, err := autostart.IsEnabled(); err == nil {
			s.cfg.AutostartEnabled = enabled
		}
		s.buildMenu()

		// Config compatibility: if both NoSleep and idle monitor are enabled,
		// resolve the conflict — NoSleep takes priority.
		if s.cfg.NoSleepEnabled && s.cfg.IdleTimeoutMinutes > 0 {
			s.cfg.IdleTimeoutMinutes = 0
		}
		s.batteryBlocked = batteryPolicyBlocks(s.cfg, power.GetStatus())
		s.reconcileRuntime()
		s.syncChecks()
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
	s.mProcessWatch = systray.AddMenuItemCheckbox(T("menu_process_watch"), "", s.cfg.ProcessWatchEnabled)
	s.registerLabel(s.mProcessWatch, "menu_process_watch")


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
	s.mThemeSwitch = systray.AddMenuItem(T("menu_theme_switch"), "")
	s.registerLabel(s.mThemeSwitch, "menu_theme_switch")

	// Enable toggle as first submenu item
	s.mThemeEnable = s.mThemeSwitch.AddSubMenuItemCheckbox(T("menu_theme_enable"), "", s.cfg.ThemeSwitchEnabled)
	s.registerLabel(s.mThemeEnable, "menu_theme_enable")

	// Light time submenu
	s.mThemeLightAt = s.mThemeSunrise.AddSubMenuItem(T("menu_theme_light_time"), "")
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
	s.mThemeDarkAt = s.mThemeSunrise.AddSubMenuItem(T("menu_theme_dark_time"), "")
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


	// Sunrise mode toggle
	s.mThemeSunrise = s.mThemeSwitch.AddSubMenuItemCheckbox(T("menu_theme_sunrise"), "", s.cfg.ThemeMode == "sunrise")
	s.registerLabel(s.mThemeSunrise, "menu_theme_sunrise")


	// Battery dark toggle
	s.mThemeBatteryDark = s.mThemeSwitch.AddSubMenuItemCheckbox(T("menu_theme_battery_dark"), "", s.cfg.ThemeDarkOnBattery)
	s.registerLabel(s.mThemeBatteryDark, "menu_theme_battery_dark")

	// Skip fullscreen toggle
	s.mThemeSkipFullscreen = s.mThemeSwitch.AddSubMenuItemCheckbox(T("menu_theme_skip_fullscreen"), "", s.cfg.ThemeSkipFullscreen)
	s.registerLabel(s.mThemeSkipFullscreen, "menu_theme_skip_fullscreen")

	systray.AddSeparator()
	s.mHotkeys = systray.AddMenuItemCheckbox(T("menu_hotkeys"), "", s.cfg.HotkeysEnabled)
	s.registerLabel(s.mHotkeys, "menu_hotkeys")
	s.mAutostart = systray.AddMenuItemCheckbox(T("menu_autostart"), "", s.cfg.AutostartEnabled)
	s.registerLabel(s.mAutostart, "menu_autostart")

	mOpenCfg := systray.AddMenuItem(T("menu_open_config"), "")
	s.registerLabel(mOpenCfg, "menu_open_config")

	// Language submenu
	mLang := systray.AddMenuItem(T("menu_language"), "")
	s.registerLabel(mLang, "menu_language")
	mLangEn := mLang.AddSubMenuItemCheckbox(T("menu_lang_en"), "", s.cfg.Language != "zh-CN")
	s.registerLabel(mLangEn, "menu_lang_en")
	mLangZh := mLang.AddSubMenuItemCheckbox(T("menu_lang_zh"), "", s.cfg.Language == "zh-CN")
	s.registerLabel(mLangZh, "menu_lang_zh")
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
				s.post(func() {
					if err := actions.Sleep(); err != nil {
						s.showError("menu_sleep", err)
					}
				})
			case <-s.mHibernate.ClickedCh:
				s.post(func() {
					if err := actions.Hibernate(); err != nil {
						s.showError("menu_hibernate", err)
					}
				})
			case <-s.mShutdown.ClickedCh:
				s.post(func() {
					if err := actions.Shutdown(); err != nil {
						s.showError("menu_shutdown", err)
					}
				})
			case <-s.mLock.ClickedCh:
				s.post(func() {
					if err := actions.Lock(); err != nil {
						s.showError("menu_lock", err)
					}
				})
			case <-s.mProcessWatch.ClickedCh:
					if s.mProcessWatch.Checked() {
						s.mProcessWatch.Uncheck()
						s.cfg.ProcessWatchEnabled = false
						s.stopProcessWatcher()
					} else {
						s.mProcessWatch.Check()
						s.cfg.ProcessWatchEnabled = true
						s.startProcessWatcher()
					}
					config.Save(s.cfg)


			config.Save(s.cfg)

			case <-s.mNoSleepEnable.ClickedCh:
					s.toggleNoSleep()

			case <-s.mIdleEnable.ClickedCh:
				s.post(func() {
					if s.cfg.IdleTimeoutMinutes > 0 {
						s.cfg.IdleTimeoutMinutes = 0
					} else {
						s.cfg.NoSleepEnabled = false
						s.cfg.IdleTimeoutMinutes = 30
					}
					s.reconcileRuntime()
					s.syncChecks()
					s.updateIcon()
					s.saveConfig()
				})

			case <-s.mAutostart.ClickedCh:
				s.post(func() {
					enable := !s.cfg.AutostartEnabled
					var err error
					if enable {
						err = autostart.Enable()
					} else {
						err = autostart.Disable()
					}
					if err != nil {
						s.showError("menu_autostart", err)
						return
					}
					s.cfg.AutostartEnabled = enable
					if enable {
						s.mAutostart.Check()
					} else {
						s.mAutostart.Uncheck()
					}
					s.saveConfig()
				})

			case <-s.mThemeSwitchNow.ClickedCh:
				s.post(func() {
					cur := themeswitch.Current()
					target := themeswitch.ModeDark
					if cur == themeswitch.ModeDark {
						target = themeswitch.ModeLight
					}
					if err := themeswitch.Switch(target); err != nil {
						s.showError("menu_theme_switch_now", err)
					}
				})

			case <-s.mThemeSunrise.ClickedCh:
			if s.mThemeSunrise.Checked() {
				s.mThemeSunrise.Uncheck()
				s.cfg.ThemeMode = "fixed"
				s.mThemeLightAt.Show()
				s.mThemeDarkAt.Show()
			} else {
				s.mThemeSunrise.Check()
				s.cfg.ThemeMode = "sunrise"
				s.mThemeLightAt.Hide()
				s.mThemeDarkAt.Hide()
			}
			s.stopThemeScheduler()
			if s.cfg.ThemeSwitchEnabled { s.startThemeScheduler() }
			config.Save(s.cfg)

			case <-s.mThemeBatteryDark.ClickedCh:
				if s.mThemeBatteryDark.Checked() {
					s.mThemeBatteryDark.Uncheck()
					s.cfg.ThemeDarkOnBattery = false
				} else {
					s.mThemeBatteryDark.Check()
					s.cfg.ThemeDarkOnBattery = true
				}
			s.stopThemeScheduler()
			if s.cfg.ThemeSwitchEnabled { s.startThemeScheduler() }
			config.Save(s.cfg)

			case <-s.mThemeSkipFullscreen.ClickedCh:
				if s.mThemeSkipFullscreen.Checked() {
					s.mThemeSkipFullscreen.Uncheck()
					s.cfg.ThemeSkipFullscreen = false
				} else {
					s.mThemeSkipFullscreen.Check()
					s.cfg.ThemeSkipFullscreen = true
				}
			s.stopThemeScheduler()
			if s.cfg.ThemeSwitchEnabled { s.startThemeScheduler() }
			config.Save(s.cfg)

		case <-s.mThemeRepair.ClickedCh:
				s.post(func() {
					if err := themeswitch.Switch(themeswitch.Current()); err != nil {
						s.showError("menu_theme_repair", err)
					}
				})

			case <-s.mThemeEnable.ClickedCh:
				s.post(func() {
					s.cfg.ThemeSwitchEnabled = !s.cfg.ThemeSwitchEnabled
					if s.cfg.ThemeSwitchEnabled {
						s.startThemeScheduler()
					} else {
						s.stopThemeScheduler()
					}
					s.syncChecks()
					s.saveConfig()
				})

			case <-s.mHotkeys.ClickedCh:
				s.post(func() {
					s.cfg.HotkeysEnabled = !s.cfg.HotkeysEnabled
					if s.cfg.HotkeysEnabled {
						s.startHotkeys()
					} else {
						s.stopHotkeys()
					}
					s.syncChecks()
					s.saveConfig()
				})

			case <-mLangEn.ClickedCh:
				s.post(func() {
					s.switchLanguage("en")
					mLangEn.Check()
					mLangZh.Uncheck()
				})

			case <-mLangZh.ClickedCh:
				s.post(func() {
					s.switchLanguage("zh-CN")
					mLangZh.Check()
					mLangEn.Uncheck()
				})

			case <-mOpenCfg.ClickedCh:
				s.post(func() {
					p, err := config.Path()
					if err == nil {
						err = exec.Command("notepad.exe", p).Start()
					}
					if err != nil {
						s.showError("menu_open_config", err)
					}
				})

			case <-mAbout.ClickedCh:
				s.post(func() { showAboutDialog(s.lang) })

			case <-mExit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()

	s.wireSubmenus()
	s.wireThemeSubmenus()

	systray.OnLeftClick = func() {
		s.showPopup()
	}
}

func (s *trayState) wireSubmenus() {
	for i, item := range s.timeoutItems {
		idx := i
		it := item
		go func() {
			for range it.ClickedCh {
				s.post(func() {
					s.cfg.IdleTimeoutMinutes = timeoutOptions[idx].minutes
					s.cfg.NoSleepEnabled = false
					s.updateTimeoutChecks()
					s.reconcileRuntime()
					s.syncChecks()
					s.updateIcon()
					s.saveConfig()
				})
			}
		}()
	}
	for i, item := range s.actionItems {
		idx := i
		it := item
		go func() {
			for range it.ClickedCh {
				s.post(func() {
					s.cfg.IdleAction = actionOptions[idx].action
					s.updateActionChecks()
					if s.mon != nil {
						s.startMonitor()
					}
					s.saveConfig()
				})
			}
		}()
	}
}

// ---- state sync -------------------------------------------------------

func (s *trayState) syncChecks() {
	if s.cfg.NoSleepEnabled {
		s.mNoSleepEnable.Check()
	} else {
		s.mNoSleepEnable.Uncheck()
	}
	if s.cfg.IdleTimeoutMinutes > 0 {
		s.mIdleEnable.Check()
		s.mIdleTimeout.Show()
		s.mIdleAction.Show()
	} else {
		s.mIdleEnable.Uncheck()
		s.mIdleTimeout.Hide()
		s.mIdleAction.Hide()
	}
	if s.cfg.ProcessWatchEnabled {
		s.mProcessWatch.Check()
	} else {
		s.mProcessWatch.Uncheck()
	}
	if s.cfg.HotkeysEnabled {
		s.mHotkeys.Check()
	} else {
		s.mHotkeys.Uncheck()
	}
	if s.cfg.ThemeSwitchEnabled {
		s.mThemeEnable.Check()
	} else {
		s.mThemeSwitch.Uncheck()
	}
	// Hide time submenus when sunrise mode is active
	// 日出日落模式开启时隐藏时间子菜单
	if s.cfg.ThemeMode == "sunrise" {
		s.mThemeLightAt.Hide()
		s.mThemeDarkAt.Hide()
	} else {
		s.mThemeLightAt.Show()
		s.mThemeDarkAt.Show()
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
	// Build combined tooltip showing all active features.
	nl := string(rune(10))
	tip := "IdleTrigger"
	if nosleep.IsEnabled() {
		systray.SetIcon(assets.IconActive)
		tip += nl + i18n.T(s.lang, "tooltip_nosleep")
	} else if s.cfg.IdleTimeoutMinutes > 0 {
		systray.SetIcon(assets.IconMonitor)
	} else {
		systray.SetIcon(assets.IconDefault)
	}
	if s.cfg.IdleTimeoutMinutes > 0 {
		act := i18n.T(s.lang, actionTranslationKey(s.cfg.IdleAction))
		tip += nl + fmt.Sprintf(i18n.T(s.lang, "tooltip_monitor"),
			s.cfg.IdleTimeoutMinutes, act)
	}
	if s.cfg.ThemeSwitchEnabled {
		tip += nl + i18n.T(s.lang, "tooltip_theme")
	}
	if s.cfg.HotkeysEnabled {
		tip += nl + i18n.T(s.lang, "tooltip_hotkeys")
	}
	systray.SetTooltip(tip)
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
		nosleep.Enable(s.cfg.KeepScreenOn)
		return
	}

	nosleep.Disable()
	if s.cfg.IdleTimeoutMinutes > 0 {
		if s.mon == nil {
			s.startMonitor()
		}
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
	}
	s.reconcileRuntime()
	mylog.Info("NoSleep toggled: enabled=%v keep_screen_on=%v", nosleep.IsEnabled(), nosleep.IsKeepingScreenOn())
	s.syncChecks()
	s.updateIcon()
	s.saveConfig()
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
				s.post(func() {
					mylog.Info("Process watcher: watched app detected, requesting NoSleep")
					s.processNoSleep = true
					s.reconcileRuntime()
					s.syncChecks()
					s.updateIcon()
				})
			},
			OnDisable: func() {
				s.post(func() {
					mylog.Info("Process watcher: no watched apps running, releasing NoSleep")
					s.processNoSleep = false
					s.reconcileRuntime()
					s.syncChecks()
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
	s.syncChecks()
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
		s.syncChecks()
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
		s.syncChecks()
		s.updateIcon()
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "nosleep:off":
		s.cfg.NoSleepEnabled = false
		s.reconcileRuntime()
		s.syncChecks()
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
		s.syncChecks()
		s.updateIcon()
		if err := s.saveConfigErr(); err != nil {
			return "err: " + err.Error()
		}
		return "ok"
	case "monitor:off":
		s.cfg.IdleTimeoutMinutes = 0
		s.reconcileRuntime()
		s.syncChecks()
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
		if enabled, statusErr := autostart.IsEnabled(); statusErr == nil {
			s.cfg.AutostartEnabled = enabled
			if enabled {
				s.mAutostart.Check()
			} else {
				s.mAutostart.Uncheck()
			}
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
		s.syncChecks()
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
	dialog.Info(i18n.T(lang, "app_title"), i18n.T(lang, "app_title"), text)
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
	s.applyLanguage()
	mylog.Info("Language switched: %s", lang)
	s.saveConfig()
}

func (s *trayState) applyLanguage() {
	T := func(key string) string { return i18n.T(s.lang, key) }
	for _, li := range s.labelItems {
		li.item.SetTitle(T(li.key))
	}
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

func (s *trayState) wireThemeSubmenus() {
	// Light time radio
	lightVals := []string{"06:00", "07:00", "08:00"}
	for i, item := range s.themeLightItems {
		idx := i
		it := item
		go func() {
			for range it.ClickedCh {
				s.post(func() {
					s.cfg.ThemeLightTime = lightVals[idx]
					for j, ti := range s.themeLightItems {
						if j == idx {
							ti.Check()
						} else {
							ti.Uncheck()
						}
					}
					if s.cfg.ThemeSwitchEnabled {
						s.startThemeScheduler()
					}
					s.saveConfig()
				})
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
				s.post(func() {
					s.cfg.ThemeDarkTime = darkVals[idx]
					for j, ti := range s.themeDarkItems {
						if j == idx {
							ti.Check()
						} else {
							ti.Uncheck()
						}
					}
					if s.cfg.ThemeSwitchEnabled {
						s.startThemeScheduler()
					}
					s.saveConfig()
				})
			}
		}()
	}
}


func (s *trayState) showPopup() {
	popup.Show(popup.State{
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
	}, func(action popup.Action, value int) {
		switch action {
		case popup.ActSleep:               actions.Sleep()
		case popup.ActHibernate:           actions.Hibernate()
		case popup.ActShutdown:            actions.Shutdown()
		case popup.ActLock:                actions.Lock()
		case popup.ActNoSleepToggle:       s.toggleNoSleep()
		case popup.ActProcessWatchToggle:  s.cfg.ProcessWatchEnabled = !s.cfg.ProcessWatchEnabled; s.reconcileRuntime(); s.saveConfig()
		case popup.ActIdleToggle:          if s.cfg.IdleTimeoutMinutes > 0 { s.cfg.IdleTimeoutMinutes = 0 } else { s.cfg.IdleTimeoutMinutes = 30 }; s.reconcileRuntime(); s.updateIcon(); s.saveConfig()
		case popup.ActIdleTimeout:         if value >= 0 { times := []int{5,10,30,60,120}; s.cfg.IdleTimeoutMinutes = times[value]; s.reconcileRuntime(); s.saveConfig() }
		case popup.ActIdleAction:          if value >= 0 { acts := []config.Action{config.ActionSleep,config.ActionHibernate,config.ActionShutdown,config.ActionLock}; s.cfg.IdleAction = acts[value]; s.saveConfig() }
		case popup.ActThemeToggle:         s.cfg.ThemeSwitchEnabled = !s.cfg.ThemeSwitchEnabled; if s.cfg.ThemeSwitchEnabled { s.startThemeScheduler() } else { s.stopThemeScheduler() }; s.updateIcon(); s.saveConfig()
		case popup.ActSunriseToggle:       s.cfg.ThemeMode = map[bool]string{true:"fixed",false:"sunrise"}[s.cfg.ThemeMode=="sunrise"]; s.stopThemeScheduler(); if s.cfg.ThemeSwitchEnabled { s.startThemeScheduler() }; s.saveConfig()
		case popup.ActBatteryToggle:       s.cfg.ThemeDarkOnBattery = !s.cfg.ThemeDarkOnBattery; s.saveConfig()
		case popup.ActFullscreenToggle:    s.cfg.ThemeSkipFullscreen = !s.cfg.ThemeSkipFullscreen; s.saveConfig()
		case popup.ActSwitchTheme:         cur := themeswitch.Current(); if cur == themeswitch.ModeDark { themeswitch.Switch(themeswitch.ModeLight) } else { themeswitch.Switch(themeswitch.ModeDark) }
		case popup.ActRepairTheme:         themeswitch.Switch(themeswitch.Current())
		case popup.ActHotkeyToggle:        s.cfg.HotkeysEnabled = !s.cfg.HotkeysEnabled; if s.cfg.HotkeysEnabled { s.startHotkeys() } else { s.stopHotkeys() }; s.saveConfig()
		case popup.ActAutostartToggle:     s.cfg.AutostartEnabled = !s.cfg.AutostartEnabled; if s.cfg.AutostartEnabled { autostart.Enable() } else { autostart.Disable() }; s.saveConfig()
		case popup.ActLanguage:            if value == 0 { s.switchLanguage("en") } else { s.switchLanguage("zh-CN") }
		case popup.ActConfig:              p, _ := config.Path(); exec.Command("notepad.exe", p).Start()
		case popup.ActAbout:               showAboutDialog(s.lang)
		case popup.ActExit:                systray.Quit()
		}
	}, func(key string) string { return i18n.T(s.lang, key) })
}
