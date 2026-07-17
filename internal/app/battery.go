package app

import (
	"fmt"
	"time"

	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/feature/keepawake"
	mylog "github.com/JeffioZ/idletrigger/internal/logging"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/powerstate"
)

// syncBatteryLoop keeps the periodic battery poll dormant when neither
// battery-aware feature is enabled. Windows power broadcasts still deliver
// immediate changes; this loop catches battery-percentage threshold crossings.
func (s *runtimeState) syncBatteryLoop() {
	if s.noSleepRequested() || (s.cfg.ThemeSwitchEnabled && s.themeAvailable()) {
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
	s.refreshBatteryPolicyWithStatus(powerstate.GetStatus())
}

func (s *runtimeState) refreshBatteryPolicyWithStatus(ps powerstate.Status) bool {
	blocked := batteryPolicyBlocks(s.cfg, ps)
	if s.themeSched != nil {
		s.themeSched.CheckNow()
	}
	s.refreshTrayThemeIcon()
	if blocked == s.batteryBlocked {
		return false
	}
	s.batteryBlocked = blocked
	mylog.Info("Battery policy changed: nosleep_blocked=%v reason=%s ac_line=%v battery=%v percent=%d charging=%v valid=%v",
		blocked, batteryPolicyReason(s.cfg, ps), ps.ACLine, ps.Battery, ps.Percent, ps.Charging, ps.Valid)
	s.reconcileRuntime()

	s.updateIcon()
	return true
}

func (s *runtimeState) handlePowerEvent(event uint32) {
	ps := powerstate.GetStatus()
	s.logPowerState(fmt.Sprintf("event:%s(0x%04x)", powerEventName(event), event), ps)
	changed := s.refreshBatteryPolicyWithStatus(ps)
	if isResumePowerEvent(event) && !changed {
		if s.noSleepRequested() && !s.batteryBlocked {
			mylog.Info("Power resume: reasserting effective Stay Awake request")
		}
		s.reconcileRuntime()
	}
}

func (s *runtimeState) logPowerState(source string, ps powerstate.Status) {
	mylog.Info("Power state: source=%s ac_line=%v battery=%v percent=%d charging=%v valid=%v nosleep_configured=%v wants_nosleep=%v nosleep_blocked=%v keepawake_enabled=%v keep_screen_on=%v reason=%s",
		source, ps.ACLine, ps.Battery, ps.Percent, ps.Charging, ps.Valid,
		s.cfg.NoSleepEnabled, s.noSleepRequested(), batteryPolicyBlocks(s.cfg, ps),
		keepawake.IsEnabled(), keepawake.IsKeepingScreenOn(), batteryPolicyReason(s.cfg, ps))
}

func batteryPolicyBlocks(cfg config.Config, status powerstate.Status) bool {
	if !status.Valid || !status.Battery || status.ACLine {
		return false
	}
	return !cfg.NoSleepOnBattery ||
		(status.Percent >= 0 && status.Percent < cfg.NoSleepBatteryThreshold)
}

func batteryPolicyReason(cfg config.Config, status powerstate.Status) string {
	if !status.Valid {
		return "power-status-unknown"
	}
	if status.ACLine || !status.Battery {
		return "ac-or-no-battery"
	}
	if !cfg.NoSleepOnBattery {
		return "battery-not-allowed"
	}
	if status.Percent >= 0 && status.Percent < cfg.NoSleepBatteryThreshold {
		return "battery-below-threshold"
	}
	return "battery-allowed"
}

// ---- IPC handler ------------------------------------------------------
