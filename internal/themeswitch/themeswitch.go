// Package themeswitch — auto-switches Windows between light and dark theme
// at scheduled times or sunrise/sunset via registry + WM_SETTINGCHANGE.
// 定时自动切换 Windows 浅色/深色主题（固定时间或日出日落），通过注册表实现。
package themeswitch

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const regPath = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`

// Mode is the target theme mode.
// 主题模式：浅色 / 深色。
type Mode int

const (
	ModeLight Mode = iota
	ModeDark
)

// Switch sets the Windows app and system theme.
// 切换 Windows 应用和系统主题。
func Switch(mode Mode) error {
	var val uint32
	if mode == ModeLight {
		val = 1
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	k.SetDWordValue("AppsUseLightTheme", val)
	k.SetDWordValue("SystemUsesLightTheme", val)

	user32 := windows.NewLazySystemDLL("user32.dll")
	sendMsg := user32.NewProc("SendMessageTimeoutW")
	const (
		hwndBroadcast   = 0xFFFF
		wmSettingChange = 0x001A
		smtoNormal      = 0x0000
	)
	imm, _ := windows.UTF16PtrFromString("ImmersiveColorSet")
	sendMsg.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(imm)), smtoNormal, 5000, 0)
	return nil
}

// Current returns the current theme mode.
// 返回当前主题模式。
func Current() Mode {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.QUERY_VALUE)
	if err != nil {
		return ModeLight
	}
	defer k.Close()
	val, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil || val == 0 {
		return ModeDark
	}
	return ModeLight
}

// Scheduler checks time periodically and switches theme.
// 定时器：定期检查并在适当时机切换主题。
type Scheduler struct {
	mode      string  // "fixed" or "sunrise"
	lightTime string  // HH:MM
	darkTime  string  // HH:MM
	latitude  float64
	longitude float64
	stopCh    chan struct{}
	mu        sync.Mutex
}

// NewScheduler creates a Scheduler.
func NewScheduler(mode, lightTime, darkTime string, lat, lon float64) *Scheduler {
	return &Scheduler{
		mode:      mode,
		lightTime: lightTime,
		darkTime:  darkTime,
		latitude:  lat,
		longitude: lon,
		stopCh:    make(chan struct{}),
	}
}

// Start begins the background check loop.
func (s *Scheduler) Start() { go s.loop() }

// Stop signals the loop to exit.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.check(now)
		}
	}
}

func (s *Scheduler) check(now time.Time) {
	var lightMin, darkMin int

	if s.mode == "sunrise" {
		lightMin, darkMin = sunriseSunset(now, s.latitude, s.longitude)
	} else {
		lightMin = parseTime(s.lightTime)
		darkMin = parseTime(s.darkTime)
	}
	if lightMin < 0 || darkMin < 0 {
		return
	}

	today := now.Truncate(24 * time.Hour)
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
		Switch(target)
	}
}

// sunriseSunset returns light and dark times as minutes since midnight,
// calculated using the NOAA solar calculator.
// 使用 NOAA 太阳计算器返回日出和日落时间（午夜后的分钟数）。
func sunriseSunset(t time.Time, lat, lon float64) (sunriseMinutes, sunsetMinutes int) {
	// Day of year
	doy := float64(t.YearDay())

	// Fractional year in radians
	gamma := 2 * math.Pi / 365 * (doy - 1 + float64(t.Hour()-12)/24)

	// Equation of time
	eqtime := 229.18 * (0.000075 + 0.001868*math.Cos(gamma) - 0.032077*math.Sin(gamma) -
		0.014615*math.Cos(2*gamma) - 0.040849*math.Sin(2*gamma))

	// Solar declination
	decl := 0.006918 - 0.399912*math.Cos(gamma) + 0.070257*math.Sin(gamma) -
		0.006758*math.Cos(2*gamma) + 0.000907*math.Sin(2*gamma) -
		0.002697*math.Cos(3*gamma) + 0.00148*math.Sin(3*gamma)

	// Hour angle
	latRad := lat * math.Pi / 180
	zenith := 90.833 * math.Pi / 180 // official sunrise/sunset zenith
	ha := math.Acos(math.Cos(zenith)/(math.Cos(latRad)*math.Cos(decl)) - math.Tan(latRad)*math.Tan(decl))

	// Solar noon in minutes (UTC)
	solarNoon := (720 - 4*lon - eqtime)
	// in minutes UTC
	_, offset := t.Zone()
	solarNoonLocal := solarNoon + float64(offset)/60 // convert to local minutes

	// Sunrise / sunset in local minutes
	sunrise := solarNoonLocal - ha*4*180/math.Pi
	sunset := solarNoonLocal + ha*4*180/math.Pi

	// Clamp to valid range and wrap
	sr := int(math.Round(sunrise))
	ss := int(math.Round(sunset))
	for sr < 0 {
		sr += 1440
	}
	for ss < 0 {
		ss += 1440
	}
	for sr >= 1440 {
		sr -= 1440
	}
	for ss >= 1440 {
		ss -= 1440
	}

	return sr, ss
}

func parseTime(s string) int {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return -1
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return -1
	}
	return h*60 + m
}
