// Package themeswitch auto-switches Windows between light and dark themes
// at scheduled times or sunrise/sunset via registry + WM_SETTINGCHANGE.
package themeswitch

import (
	"fmt"
	"math"
	osExec "os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/JeffioZ/idletrigger/internal/power"
)

// timezoneLookup maps Windows timezone names to approximate lat/lon.
var timezoneLookup = map[string][2]float64{
	"China Standard Time":            {39.9, 116.4},
	"Taipei Standard Time":           {25.0, 121.5},
	"Tokyo Standard Time":            {35.7, 139.7},
	"Korea Standard Time":            {37.6, 127.0},
	"Singapore Standard Time":        {1.35, 103.8},
	"India Standard Time":            {28.6, 77.2},
	"W. Europe Standard Time":        {52.5, 13.4},
	"GMT Standard Time":              {51.5, -0.1},
	"Central Europe Standard Time":   {48.2, 16.4},
	"E. Europe Standard Time":        {50.4, 30.5},
	"Russian Standard Time":          {55.8, 37.6},
	"Eastern Standard Time":          {40.7, -74.0},
	"Central Standard Time":          {41.9, -87.6},
	"Mountain Standard Time":         {33.4, -112.0},
	"Pacific Standard Time":          {34.0, -118.2},
	"Alaskan Standard Time":          {61.2, -149.9},
	"Hawaiian Standard Time":         {21.3, -157.8},
	"E. South America Standard Time": {-23.5, -46.6},
	"Atlantic Standard Time":         {-34.6, -58.4},
	"AUS Eastern Standard Time":      {-33.9, 151.2},
	"AUS Central Standard Time":      {-34.9, 138.6},
	"New Zealand Standard Time":      {-36.8, 174.8},
}

// AutoLocation returns approximate coordinates based on the Windows timezone.
// Falls back to Beijing if the timezone is unknown.

func GetLocation() (float64, float64) {
	ps, _ := osExec.LookPath("powershell.exe")
	cmd := osExec.Command(ps, "-NoProfile", "-NonInteractive", "-Command",
		"=New-Object Windows.Devices.Geolocation.Geolocator;"+
			"=.GetGeopositionAsync().GetAwaiter().GetResult();"+
			"Write-Output (.Coordinate.Point.Position.Latitude.ToString())+' '+"+
			"(.Coordinate.Point.Position.Longitude.ToString())")
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	var lat, lon float64
	if _, err := fmt.Sscanf(string(out), "%f %f", &lat, &lon); err != nil {
		return 0, 0
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0
	}
	return lat, lon
}

func AutoLocation() (float64, float64) {
	// Try GPS first, fall back to timezone.
	if lat, lon := GetLocation(); lat != 0 || lon != 0 {
		return lat, lon
	}
	// Get the Windows timezone name via GetTimeZoneInformation.
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := kernel32.NewProc("GetTimeZoneInformation")
	type tzInfo struct {
		Bias         int32
		StandardName [32]uint16
		StandardDate windows.Systemtime
		StandardBias int32
		DaylightName [32]uint16
		DaylightDate windows.Systemtime
		DaylightBias int32
	}
	var tz tzInfo
	proc.Call(uintptr(unsafe.Pointer(&tz)))
	name := windows.UTF16ToString(tz.StandardName[:])
	if coords, ok := timezoneLookup[name]; ok {
		return coords[0], coords[1]
	}
	return 39.9, 116.4
}

const regPath = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`

// Mode is the target theme mode.
type Mode int

const (
	ModeLight Mode = iota
	ModeDark
)

// Switch sets the Windows app and system theme.
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
	if err := k.SetDWordValue("AppsUseLightTheme", val); err != nil {
		return fmt.Errorf("set AppsUseLightTheme: %w", err)
	}
	if err := k.SetDWordValue("SystemUsesLightTheme", val); err != nil {
		return fmt.Errorf("set SystemUsesLightTheme: %w", err)
	}

	notifyThemeChanged()
	return nil
}

// Refresh asks Windows shell surfaces to re-read the current theme settings.
func Refresh() error {
	notifyThemeChanged()
	return nil
}

func notifyThemeChanged() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	updatePerUser := user32.NewProc("UpdatePerUserSystemParameters")
	sendNotify := user32.NewProc("SendNotifyMessageW")
	sendMsg := user32.NewProc("SendMessageTimeoutW")
	const (
		hwndBroadcast    = 0xFFFF
		wmThemeChanged   = 0x031A
		wmSysColorChange = 0x0015
		wmSettingChange  = 0x001A
		smtoAbortIfHung  = 0x0002
	)

	updatePerUser.Call(0, 1)

	// Notify every top-level window, including Explorer's taskbars on each
	// display. Windows components do not all listen for the same token, so send
	// the common theme and color notifications without blocking on hung apps.
	for _, token := range []string{"ImmersiveColorSet", "WindowsThemeElement", "Policy"} {
		ptr, _ := windows.UTF16PtrFromString(token)
		lp := uintptr(unsafe.Pointer(ptr))
		sendNotify.Call(hwndBroadcast, wmSettingChange, 0, lp)
		sendMsg.Call(hwndBroadcast, wmSettingChange, 0, lp, smtoAbortIfHung, 250, 0)
	}
	sendNotify.Call(hwndBroadcast, wmSysColorChange, 0, 0)
	sendMsg.Call(hwndBroadcast, wmSysColorChange, 0, 0, smtoAbortIfHung, 250, 0)
	sendNotify.Call(hwndBroadcast, wmThemeChanged, 0, 0)
	sendMsg.Call(hwndBroadcast, wmThemeChanged, 0, 0, smtoAbortIfHung, 250, 0)
}

// Current returns the current theme mode.
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

// ScheduleTimes returns today's light and dark switch times as HH:MM.
func ScheduleTimes(mode, lightTime, darkTime string, lat, lon float64, now time.Time) (string, string, bool) {
	var lightMin, darkMin int
	if mode == "sunrise" {
		lightMin, darkMin = sunriseSunset(now, lat, lon)
	} else {
		lightMin = parseTime(lightTime)
		darkMin = parseTime(darkTime)
	}
	if lightMin < 0 || darkMin < 0 {
		return "", "", false
	}
	return formatMinute(lightMin), formatMinute(darkMin), true
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
	manualUntil    atomic.Int64
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
	// If dark-on-battery is enabled and running on battery, force dark.
	if s.darkOnBattery && onBattery() {
		if Current() != ModeDark && (!s.skipFullscreen || !IsFullscreen()) {
			_ = Switch(ModeDark)
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
		Switch(target)
	}
}

func (s *Scheduler) scheduleMinutes(now time.Time) (int, int) {
	if s.mode == "sunrise" {
		return sunriseSunset(now, s.latitude, s.longitude)
	}
	return parseTime(s.lightTime), parseTime(s.darkTime)
}

// sunriseSunset returns light and dark times as minutes since midnight,
// calculated using the NOAA solar calculator.
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
	acosArg := math.Cos(zenith)/(math.Cos(latRad)*math.Cos(decl)) - math.Tan(latRad)*math.Tan(decl)
	if acosArg < -1 || acosArg > 1 {
		return -1, -1
	}
	ha := math.Acos(acosArg)

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

// IsFullscreen returns true when the foreground window covers its monitor.
func IsFullscreen() bool {
	user32 := windows.NewLazySystemDLL("user32.dll")
	getForeground := user32.NewProc("GetForegroundWindow")
	hwnd, _, _ := getForeground.Call()

	if hwnd == 0 {
		return false
	}

	type rect struct{ left, top, right, bottom int32 }
	var r rect
	getWindowRect := user32.NewProc("GetWindowRect")
	if ok, _, _ := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return false
	}

	monitorFromWindow := user32.NewProc("MonitorFromWindow")
	const monitorDefaultToNearest = 2
	monitor, _, _ := monitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if monitor == 0 {
		return false
	}
	type monitorInfo struct {
		size    uint32
		monitor rect
		work    rect
		flags   uint32
	}
	var info monitorInfo
	info.size = uint32(unsafe.Sizeof(info))
	getMonitorInfo := user32.NewProc("GetMonitorInfoW")
	if ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return false
	}

	const tolerance = 2
	return r.left <= info.monitor.left+tolerance &&
		r.top <= info.monitor.top+tolerance &&
		r.right >= info.monitor.right-tolerance &&
		r.bottom >= info.monitor.bottom-tolerance
}

func onBattery() bool {
	return power.OnBattery()
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
