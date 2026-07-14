// Package theme auto-switches Windows between light and dark themes
// at scheduled times or sunrise/sunset via registry + WM_SETTINGCHANGE.
package theme

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mylog "github.com/JeffioZ/idletrigger/internal/logging"
)

type ScheduleInfo struct {
	LightTime     string
	DarkTime      string
	OK            bool
	FixedFallback bool
}

// ScheduleTimes returns today's light and dark switch times as HH:MM.
func ScheduleTimes(mode, lightTime, darkTime string, lat, lon float64, now time.Time) (string, string, bool) {
	info := ScheduleInfoFor(mode, lightTime, darkTime, lat, lon, now)
	return info.LightTime, info.DarkTime, info.OK
}

// ScheduleInfoFor returns the theme schedule and reports when sunrise/sunset
// mode had to fall back to fixed times, such as during polar day/night.
func ScheduleInfoFor(mode, lightTime, darkTime string, lat, lon float64, now time.Time) ScheduleInfo {
	var lightMin, darkMin int
	if mode == "sunrise" {
		lightMin, darkMin = CalcSunriseSunset(now, lat, lon)
		if lightMin >= 0 && darkMin >= 0 {
			return ScheduleInfo{LightTime: formatMinute(lightMin), DarkTime: formatMinute(darkMin), OK: true}
		}
	}

	lightMin = parseTime(lightTime)
	darkMin = parseTime(darkTime)
	if lightMin < 0 || darkMin < 0 {
		return ScheduleInfo{}
	}
	return ScheduleInfo{
		LightTime:     formatMinute(lightMin),
		DarkTime:      formatMinute(darkMin),
		OK:            true,
		FixedFallback: mode == "sunrise",
	}
}

// Scheduler checks time periodically and switches theme.
type Scheduler struct {
	mode           string // "fixed" or "sunrise"
	lightTime      string // HH:MM
	darkTime       string // HH:MM
	latitude       float64
	longitude      float64
	skipFullscreen bool
	darkOnBattery  bool
	stopCh         chan struct{}
	doneCh         chan struct{}
	running        bool
	mu             sync.Mutex
	checkMu        sync.Mutex
	manualUntil    atomic.Int64
	logMu          sync.Mutex
	lastSwitchErr  string
}

// NewScheduler creates a Scheduler.
func NewScheduler(mode, lightTime, darkTime string, lat, lon float64, skipFullscreen, darkOnBattery bool) *Scheduler {
	return &Scheduler{
		mode:           mode,
		lightTime:      lightTime,
		darkTime:       darkTime,
		latitude:       lat,
		longitude:      lon,
		skipFullscreen: skipFullscreen,
		darkOnBattery:  darkOnBattery,
	}
}

// Start begins the background check loop.
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.running = true
	go s.loop(s.stopCh, s.doneCh)
}

// Stop signals the loop to exit.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	stopCh := s.stopCh
	doneCh := s.doneCh
	s.running = false
	close(stopCh)
	s.mu.Unlock()
	<-doneCh
}

func (s *Scheduler) loop(stopCh <-chan struct{}, doneCh chan<- struct{}) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	defer close(doneCh)
	s.check(time.Now())
	for {
		select {
		case <-stopCh:
			return
		case now := <-ticker.C:
			s.check(now)
		}
	}
}

// CheckNow evaluates the current power/time policy immediately. It is used
// when Windows power state changes so dark-on-battery does not wait for the
// regular scheduler tick.
func (s *Scheduler) CheckNow() {
	if s == nil {
		return
	}
	s.check(time.Now())
}

// HoldManualOverride keeps a user-selected theme until the next scheduled
// light or dark transition. Battery dark-mode policy remains authoritative.
func (s *Scheduler) HoldManualOverride(now time.Time) {
	if s == nil {
		return
	}
	lightMin, darkMin := s.scheduleMinutes(now)
	if lightMin < 0 || darkMin < 0 {
		return
	}
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	var next time.Time
	for _, candidate := range []time.Time{
		today.Add(time.Duration(lightMin) * time.Minute),
		today.Add(time.Duration(darkMin) * time.Minute),
	} {
		if !candidate.After(now) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	s.manualUntil.Store(next.UnixNano())
}

func (s *Scheduler) check(now time.Time) {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	// If dark-on-battery is enabled and running on battery, force dark.
	if s.darkOnBattery && onBattery() {
		if Current() != ModeDark && (!s.skipFullscreen || !IsFullscreen()) {
			if err := Switch(ModeDark); err != nil {
				s.logSwitchFailure("battery", ModeDark, err)
			} else {
				s.clearSwitchFailure()
			}
		}
		return
	}
	lightMin, darkMin := s.scheduleMinutes(now)
	if lightMin < 0 || darkMin < 0 {
		return
	}
	if until := s.manualUntil.Load(); until > now.UnixNano() {
		return
	}

	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	lightToday := today.Add(time.Duration(lightMin) * time.Minute)
	darkToday := today.Add(time.Duration(darkMin) * time.Minute)

	var target Mode
	if lightToday.Before(darkToday) {
		if now.After(lightToday) && now.Before(darkToday) {
			target = ModeLight
		} else {
			target = ModeDark
		}
	} else {
		if now.After(darkToday) || now.Before(lightToday) {
			target = ModeDark
		} else {
			target = ModeLight
		}
	}

	if Current() != target {
		// Skip switch during fullscreen apps/games if configured.
		if s.skipFullscreen && IsFullscreen() {
			return
		}
		if err := Switch(target); err != nil {
			s.logSwitchFailure("schedule", target, err)
		} else {
			s.clearSwitchFailure()
		}
	}
}

func (s *Scheduler) logSwitchFailure(reason string, target Mode, err error) {
	key := fmt.Sprintf("%s|%s|%v", reason, themeModeName(target), err)
	s.logMu.Lock()
	if s.lastSwitchErr == key {
		s.logMu.Unlock()
		return
	}
	s.lastSwitchErr = key
	s.logMu.Unlock()
	mylog.Info("Theme switch failed: reason=%s target=%s error=%v", reason, themeModeName(target), err)
}

func (s *Scheduler) clearSwitchFailure() {
	s.logMu.Lock()
	s.lastSwitchErr = ""
	s.logMu.Unlock()
}

func themeModeName(mode Mode) string {
	if mode == ModeDark {
		return "dark"
	}
	return "light"
}

func (s *Scheduler) scheduleMinutes(now time.Time) (int, int) {
	if s.mode == "sunrise" {
		lightMin, darkMin := CalcSunriseSunset(now, s.latitude, s.longitude)
		if lightMin >= 0 && darkMin >= 0 {
			return lightMin, darkMin
		}
	}
	return parseTime(s.lightTime), parseTime(s.darkTime)
}

func parseTime(s string) int {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return -1
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return -1
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return -1
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return -1
	}
	return h*60 + m
}

func formatMinute(minute int) string {
	for minute < 0 {
		minute += 1440
	}
	minute %= 1440
	return fmt.Sprintf("%02d:%02d", minute/60, minute%60)
}
