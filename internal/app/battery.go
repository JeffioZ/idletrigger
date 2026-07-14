package app

import (
	"github.com/JeffioZ/idletrigger/internal/config"
	mylog "github.com/JeffioZ/idletrigger/internal/logging"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/powerstate"
	"time"
)

// syncBatteryLoop keeps the periodic battery poll dormant when neither
// battery-aware feature is enabled. Windows power broadcasts still deliver
// immediate changes; this loop catches battery-percentage threshold crossings.
func (s *runtimeState) syncBatteryLoop() {
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

func (s *runtimeState) stopBatteryLoop() {
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
func (s *runtimeState) batteryLoop(stopCh <-chan struct{}, doneCh chan<- struct{}) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	defer close(doneCh)
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			select {
			case s.requestCh <- runtimeRequest{fn: func() string {
				s.refreshBatteryPolicy()
				return ""
			}}:
			case <-stopCh:
				return
			}
		}
	}
}

func (s *runtimeState) refreshBatteryPolicy() {
	ps := powerstate.GetStatus()
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

func batteryPolicyBlocks(cfg config.Config, status powerstate.Status) bool {
	if !status.Valid || !status.Battery || status.ACLine {
		return false
	}
	return !cfg.NoSleepOnBattery ||
		(status.Percent >= 0 && status.Percent < cfg.NoSleepBatteryThreshold)
}

// ---- IPC handler ------------------------------------------------------
