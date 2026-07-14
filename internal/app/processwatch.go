package app

import (
	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/feature/processwatch"
	mylog "github.com/JeffioZ/idletrigger/internal/logging"
	"strings"
)

func (s *runtimeState) startProcessWatcher() {
	s.stopProcessWatcher()
	list := effectiveProcessWatchList(s.cfg)
	if len(list) == 0 {
		mylog.Info("Process condition inactive: process_watch_list is empty; Stay Awake is not process-limited")
		return
	}
	mylog.Info("Process watcher started: exes=%s", strings.Join(list, ","))
	s.procWatch = processwatch.New(list,
		processwatch.Callbacks{
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

func (s *runtimeState) stopProcessWatcher() {
	if s.procWatch != nil {
		s.procWatch.Stop()
		s.procWatch = nil
		mylog.Info("Process watcher stopped")
	}
	s.processNoSleep = false
}

func (s *runtimeState) syncProcessWatcher() {
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
