package app

import (
	"github.com/JeffioZ/idletrigger/internal/config"
	mylog "github.com/JeffioZ/idletrigger/internal/logging"
	"os"
	"path/filepath"
	"time"
)

func (s *runtimeState) applyLogging() {
	if s.cfg.LoggingEnabled {
		exePath, _ := os.Executable()
		mylog.Init(true, filepath.Dir(exePath))
		mylog.Info("Debug logging enabled from control panel")
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
