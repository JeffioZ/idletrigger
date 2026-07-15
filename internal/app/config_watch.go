package app

import (
	"os"
	"path/filepath"
	"time"

	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/feature/keepawake"
	mylog "github.com/JeffioZ/idletrigger/internal/logging"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/powerstate"
)

func (s *runtimeState) applyLogging() {
	if s.cfg.LoggingEnabled {
		exePath, _ := os.Executable()
		mylog.Init(true, filepath.Dir(exePath))
		mylog.Info("Debug logging enabled from control panel")
		ps := powerstate.GetStatus()
		s.logPowerState("logging-enabled", ps)
		mylog.Info("Runtime snapshot: nosleep_configured=%v process_watch_enabled=%v process_list_count=%d process_match=%v wants_nosleep=%v battery_blocked=%v keepawake_enabled=%v keep_screen_on=%v idle_timeout_min=%d monitor_running=%v",
			s.cfg.NoSleepEnabled, s.cfg.ProcessWatchEnabled, len(effectiveProcessWatchList(s.cfg)), s.processNoSleep,
			noSleepRequested(s.cfg, s.processNoSleep), batteryPolicyBlocks(s.cfg, ps), keepawake.IsEnabled(),
			keepawake.IsKeepingScreenOn(), s.cfg.IdleTimeoutMinutes, s.mon != nil)
		return
	}
	mylog.Info("Debug logging disabled from control panel")
	mylog.Close()
}

func (s *runtimeState) watchConfig() {
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

func (s *runtimeState) rememberConfigModTime() {
	path, err := config.Path()
	if err != nil {
		return
	}
	if info, err := os.Stat(path); err == nil {
		s.selfConfigMod.Store(info.ModTime().UnixNano())
	}
}
