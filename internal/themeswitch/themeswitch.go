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


// timezoneLookup maps Windows timezone names to approximate lat/lon.
// 时区查找表：Windows 时区名 → 近似经纬度。
var timezoneLookup = map[string][2]float64{
	"China Standard Time":           {39.9, 116.4},
	"Taipei Standard Time":          {25.0, 121.5},
	"Tokyo Standard Time":           {35.7, 139.7},
	"Korea Standard Time":           {37.6, 127.0},
	"Singapore Standard Time":       {1.35, 103.8},
	"India Standard Time":           {28.6, 77.2},
	"W. Europe Standard Time":       {52.5, 13.4},
	"GMT Standard Time":             {51.5, -0.1},
	"Central Europe Standard Time":  {48.2, 16.4},
	"E. Europe Standard Time":       {50.4, 30.5},
	"Russian Standard Time":         {55.8, 37.6},
	"Eastern Standard Time":         {40.7, -74.0},
	"Central Standard Time":         {41.9, -87.6},
	"Mountain Standard Time":        {33.4, -112.0},
	"Pacific Standard Time":         {34.0, -118.2},
	"Alaskan Standard Time":         {61.2, -149.9},
	"Hawaiian Standard Time":        {21.3, -157.8},
	"E. South America Standard Time":{-23.5, -46.6},
	"Atlantic Standard Time":        {-34.6, -58.4},
	"AUS Eastern Standard Time":     {-33.9, 151.2},
	"AUS Central Standard Time":     {-34.9, 138.6},
	"New Zealand Standard Time":     {-36.8, 174.8},
}

// AutoLocation returns approximate coordinates based on the Windows timezone.
// Falls back to Beijing if the timezone is unknown.
// 根据 Windows 时区返回近似经纬度，未识别时区回退到北京。
func AutoLocation() (float64, float64) {
	// Get the Windows timezone name via GetTimeZoneInformation.
	// 通过 GetTimeZoneInformation 获取 Windows 时区名称。
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
	longitude       float64
	skipFullscreen  bool
	darkOnBattery   bool
	stopCh         chan struct{}
	mu        sync.Mutex
}

// NewScheduler creates a Scheduler.
func NewScheduler(mode, lightTime, darkTime string, lat, lon float64, skipFullscreen, darkOnBattery bool) *Scheduler {
	return &Scheduler{
		mode:      mode,
		lightTime: lightTime,
		darkTime:  darkTime,
		latitude:  lat,
		longitude:      lon,
		skipFullscreen: skipFullscreen,
		darkOnBattery:  darkOnBattery,
		stopCh:         make(chan struct{}),
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
	// If dark-on-battery is enabled and running on battery, force dark.
	// 电池深色模式：使用电池时强制深色。
	if s.darkOnBattery && onBattery() && Current() != ModeDark {
		if !s.skipFullscreen || !IsFullscreen() {
			Switch(ModeDark)
			return
		}
	}
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
		// 如果配置了全屏时不切换，则跳过。
		if s.skipFullscreen && IsFullscreen() {
			return
		}
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
	// Clamp to [-1, 1] to avoid NaN in polar regions.
	// 限制在 [-1, 1] 避免极昼/极夜 NaN。
	acosArg := math.Cos(zenith)/(math.Cos(latRad)*math.Cos(decl)) - math.Tan(latRad)*math.Tan(decl)
	if acosArg < -1 { acosArg = -1 }
	if acosArg > 1 { acosArg = 1 }
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

// IsFullscreen returns true when the foreground window covers the entire
// primary monitor (likely a game or fullscreen video).
// 检测前台窗口是否全屏（可能是游戏或全屏视频）。
func IsFullscreen() bool {
	user32 := windows.NewLazySystemDLL("user32.dll")
	getForeground := user32.NewProc("GetForegroundWindow")
	hwnd, _, _ := getForeground.Call()

	if hwnd == 0 {
		return false
	}

	// Get window rect
	type rect struct{ left, top, right, bottom int32 }
	var r rect
	getWindowRect := user32.NewProc("GetWindowRect")
	getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))

	// Get primary monitor dimensions
	getSystemMetrics := user32.NewProc("GetSystemMetrics")
	const (
		smCxScreen = 0
		smCyScreen = 1
	)
	sw, _, _ := getSystemMetrics.Call(smCxScreen)
	sh, _, _ := getSystemMetrics.Call(smCyScreen)

	// Check if window covers the entire screen
	w := r.right - r.left
	h := r.bottom - r.top
	return int32(sw) <= w && int32(sh) <= h
}


func onBattery() bool {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := kernel32.NewProc("GetSystemPowerStatus")
	type sps struct {
		ACLineStatus       byte
		BatteryFlag        byte
		BatteryLifePercent byte
		_                  byte
		_                  uint32
		_                  uint32
	}
	var s sps
	proc.Call(uintptr(unsafe.Pointer(&s)))
	return s.ACLineStatus == 0 && s.BatteryFlag != 128
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
