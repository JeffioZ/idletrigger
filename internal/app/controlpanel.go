package app

import (
	"github.com/JeffioZ/idletrigger/internal/i18n"
	mylog "github.com/JeffioZ/idletrigger/internal/logging"
	"github.com/JeffioZ/idletrigger/internal/ui/controlpanel"
	"github.com/JeffioZ/idletrigger/internal/ui/trayicon"
	"github.com/JeffioZ/idletrigger/internal/version"
)

type controlPanelSnapshot struct {
	state controlpanel.State
	lang  string
}

func (s *runtimeState) showControlPanel() {
	s.openControlPanel(false)
}

func (s *runtimeState) refreshControlPanel() {
	s.openControlPanel(true)
}

func (s *runtimeState) openControlPanel(refresh bool) {
	// Snapshot state off the Win32 message thread. This keeps the UI loop free
	// while the serialized state queue is busy stopping a monitor or applying a
	// config reload, and avoids a UI -> state -> worker -> UI wait cycle.
	go s.prepareControlPanel(refresh)
}

func (s *runtimeState) prepareControlPanel(refresh bool) {
	result := make(chan controlPanelSnapshot, 1)
	s.requestCh <- runtimeRequest{fn: func() string {
		result <- controlPanelSnapshot{
			state: controlpanel.State{
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
				Owner:                   trayicon.WindowHandle(),
				DeveloperCapturePanel:   s.devtools.CapturePanel,
				DeveloperWarningPreview: s.devtools.WarningPreview,
			},
			lang: s.lang,
		}
		return ""
	}}
	snapshot := <-result
	onAction := func(action controlpanel.Action, value int) {
		s.post(func() { s.handleControlPanelAction(action, value) })
	}
	langFn := func(key string) string { return i18n.T(snapshot.lang, key) }
	trayicon.Post(func() {
		var err error
		if refresh {
			err = controlpanel.Refresh(snapshot.state, onAction, langFn)
		} else {
			err = controlpanel.Show(snapshot.state, onAction, langFn)
		}
		if err != nil {
			mylog.Info("Control panel open failed: %v", err)
		}
	})
}
