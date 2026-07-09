// Package themeswitch — auto-switches Windows between light and dark theme
// at scheduled times via registry + WM_SETTINGCHANGE broadcast.
// 定时自动切换 Windows 浅色/深色主题，通过注册表 + WM_SETTINGCHANGE 广播实现。
package themeswitch

import (
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	regPath = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
)

// Mode is the target theme mode.
type Mode int

const (
	ModeLight Mode = iota
	ModeDark
)

// Switch sets the Windows app and system theme to light or dark.
// 将 Windows 应用和系统主题设置为浅色或深色。
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

	// Broadcast WM_SETTINGCHANGE so the shell applies the change immediately.
	// 广播 WM_SETTINGCHANGE 使 Shell 立即应用更改。
	user32 := windows.NewLazySystemDLL("user32.dll")
	sendMsg := user32.NewProc("SendMessageTimeoutW")
	const (
		hwndBroadcast   = 0xFFFF
		wmSettingChange = 0x001A
		smtoNormal      = 0x0000
	)
	imm, _ := windows.UTF16PtrFromString("ImmersiveColorSet")
	sendMsg.Call(
		hwndBroadcast, wmSettingChange, 0,
		uintptr(unsafe.Pointer(imm)),
		smtoNormal, 5000, 0,
	)
	return nil
}

// Current returns the current theme mode (Light or Dark).
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

// Scheduler checks time periodically and switches theme at configured times.
// 定时器：定期检查时间，在配置的时间点切换主题。
type Scheduler struct {
	lightTime string // "HH:MM"
	darkTime  string // "HH:MM"
	stopCh    chan struct{}
	mu        sync.Mutex
}

// NewScheduler creates a Scheduler.  lightTime and darkTime are "HH:MM" strings.
func NewScheduler(lightTime, darkTime string) *Scheduler {
	return &Scheduler{
		lightTime: lightTime,
		darkTime:  darkTime,
		stopCh:    make(chan struct{}),
	}
}

// Start begins the background check loop.
func (s *Scheduler) Start() {
	go s.loop()
}

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

	// Don't switch immediately — wait for the first tick.
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
	light := parseTime(s.lightTime)
	dark := parseTime(s.darkTime)
	if light < 0 || dark < 0 {
		return
	}

	today := now.Truncate(24 * time.Hour)
	lightToday := today.Add(time.Duration(light) * time.Minute)
	darkToday := today.Add(time.Duration(dark) * time.Minute)

	// Determine which mode we should be in based on current time.
	// 根据当前时间判断应该处于哪个模式。
	var target Mode
	if lightToday.Before(darkToday) {
		// Light comes first (e.g. 07:00 light, 19:00 dark)
		if now.After(lightToday) && now.Before(darkToday) {
			target = ModeLight
		} else {
			target = ModeDark
		}
	} else {
		// Dark comes first (e.g. 19:00 dark, 07:00 light — overnight)
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

// parseTime parses "HH:MM" into minutes since midnight, or -1 on error.
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
