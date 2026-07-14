// Package config reads and writes IdleTrigger's TOML configuration file.
// The config file (IdleTrigger.toml) lives next to the executable and is
// plain-text TOML — users can edit it directly.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/JeffioZ/idletrigger/internal/i18n"
)

// Action is a system power action.
type Action string

const (
	ActionSleep     Action = "sleep"
	ActionHibernate Action = "hibernate"
	ActionShutdown  Action = "shutdown"
	ActionLock      Action = "lock"

	DefaultIdleTimeoutMinutes = 30
)

var idleActions = [...]Action{ActionSleep, ActionHibernate, ActionShutdown, ActionLock}

// ValidIdleAction reports whether action can be used by the idle monitor.
func ValidIdleAction(action Action) bool {
	return IdleActionIndex(action) >= 0
}

// IdleActionAt returns the idle-monitor action at the stable UI index.
func IdleActionAt(index int) (Action, bool) {
	if index < 0 || index >= len(idleActions) {
		return "", false
	}
	return idleActions[index], true
}

// IdleActionIndex returns the stable UI index for an idle-monitor action.
func IdleActionIndex(action Action) int {
	for index, candidate := range idleActions {
		if action == candidate {
			return index
		}
	}
	return -1
}

const configTemplateVersion = 8

// Config holds all user-configurable settings.
type Config struct {
	// Language for UI strings: "auto" (follow OS), "en", "zh-CN".
	Language string `toml:"language"`

	// IdleTimeoutMinutes is the idle time in minutes before the selected
	// action runs after no keyboard or mouse input. 0 disables the monitor.
	IdleTimeoutMinutes int `toml:"idle_timeout_minutes"`

	// IdleAction is the system action to run after the idle time is reached.
	IdleAction Action `toml:"idle_action"`

	// IdleWarningSeconds controls the in-app warning shown before the idle
	// action fires. 0 disables it for silent operation. Default: 30.
	IdleWarningSeconds int `toml:"idle_warning_seconds"`

	// IdleEnhancedMonitor enables enhanced idle monitoring when Windows idle
	// time is refreshed by a stable low-frequency source. Default: false.
	IdleEnhancedMonitor bool `toml:"idle_enhanced_monitor"`

	// NoSleepEnabled prevents the system from sleeping automatically.
	NoSleepEnabled bool `toml:"nosleep_enabled"`

	// KeepScreenOn prevents the display from turning off while NoSleep is active.
	KeepScreenOn bool `toml:"keep_screen_on"`

	// NoSleepOnBattery allows NoSleep to remain active when running on
	// battery power.  When false, NoSleep is auto-disabled on battery.
	NoSleepOnBattery bool `toml:"nosleep_on_battery"`

	// NoSleepBatteryThreshold is the minimum battery percentage below which
	// NoSleep is forced off (0–100, default 20).
	NoSleepBatteryThreshold int `toml:"nosleep_battery_threshold"`

	// HotkeysEnabled enables global keyboard shortcuts.
	HotkeysEnabled bool `toml:"hotkeys_enabled"`

	// ProcessWatchEnabled limits Stay Awake to applicable processes when Stay
	// Awake is enabled. If no listed process is running, no keep-awake request
	// is made. It does not activate Stay Awake by itself.
	ProcessWatchEnabled bool `toml:"process_watch_enabled"`

	// ProcessWatchList is a list of applicable case-insensitive .exe names.
	// Empty means Stay Awake is not process-limited.
	ProcessWatchList []string `toml:"process_watch_list"`

	// LoggingEnabled writes debug logs to IdleTrigger.log.
	LoggingEnabled bool `toml:"logging_enabled"`

	// ThemeSwitchEnabled auto-switches Windows theme at scheduled times.
	ThemeSwitchEnabled bool `toml:"theme_switch_enabled"`

	// ThemeLightTime is when to switch to light mode (HH:MM).
	ThemeLightTime string `toml:"theme_light_time"`

	// ThemeDarkTime is when to switch to dark mode (HH:MM).
	ThemeDarkTime string `toml:"theme_dark_time"`

	// ThemeMode: "fixed" (use times above) or "sunrise" (calculate from location).
	ThemeMode string `toml:"theme_mode"`

	// ThemeLatitude / ThemeLongitude override automatic sunrise/sunset location.
	ThemeLatitude  float64 `toml:"theme_latitude"`
	ThemeLongitude float64 `toml:"theme_longitude"`

	// ThemeIPLocationEnabled allows an HTTPS IP geolocation lookup when
	// sunrise/sunset mode has no explicit coordinates.
	ThemeIPLocationEnabled bool `toml:"theme_ip_location_enabled"`

	// ThemeDarkOnBattery switches to dark mode when on battery.
	ThemeDarkOnBattery bool `toml:"theme_dark_on_battery"`

	// ThemeSkipFullscreen prevents switching during fullscreen apps/games.
	ThemeSkipFullscreen bool `toml:"theme_skip_fullscreen"`

	// AutostartEnabled mirrors the current user's registry Run entry. It is
	// runtime state rather than a TOML setting.
	AutostartEnabled bool `toml:"-"`
}

// DefaultConfig returns the factory-default configuration.
func DefaultConfig() Config {
	return Config{
		Language:                "auto",
		IdleTimeoutMinutes:      DefaultIdleTimeoutMinutes,
		IdleAction:              ActionSleep,
		IdleWarningSeconds:      30,
		IdleEnhancedMonitor:     false,
		NoSleepEnabled:          false,
		KeepScreenOn:            false,
		NoSleepOnBattery:        false,
		NoSleepBatteryThreshold: 20,
		HotkeysEnabled:          false,
		ProcessWatchEnabled:     false,
		ProcessWatchList:        nil,
		LoggingEnabled:          false,
		ThemeSwitchEnabled:      false,
		ThemeLightTime:          "07:00",
		ThemeDarkTime:           "19:00",
		ThemeMode:               "sunrise",
		ThemeLatitude:           0,
		ThemeLongitude:          0,
		ThemeIPLocationEnabled:  false,
		ThemeDarkOnBattery:      true,
		ThemeSkipFullscreen:     true,
		AutostartEnabled:        false,
	}
}

// Path returns the full path to the config file next to the executable.
func Path() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot locate executable: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), "IdleTrigger.toml"), nil
}

// Load reads and parses the config file.  If the file does not exist the
// factory defaults are returned and the file is created.
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return DefaultConfig(), err
	}
	return loadFrom(p)
}

func loadFrom(p string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if saveErr := saveTo(p, cfg); saveErr != nil {
				return cfg, saveErr
			}
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("parse config: %w", err)
	}
	normalized := NormalizeConfig(cfg)
	corrected := !reflect.DeepEqual(normalized, cfg)
	if corrected {
		if err := os.WriteFile(p+".bak", data, 0600); err != nil {
			return normalized, fmt.Errorf("back up corrected config: %w", err)
		}
	}
	if needsAnnotatedTOMLRefresh(data) || corrected {
		if err := saveTo(p, normalized); err != nil {
			return normalized, fmt.Errorf("refresh config annotations: %w", err)
		}
	}
	return normalized, nil
}

func needsAnnotatedTOMLRefresh(data []byte) bool {
	text := string(data)
	if !strings.Contains(text, configTemplateVersionMarker()) {
		return true
	}
	for _, marker := range []string{
		"# -- 保持唤醒 / Stay Awake --",
		"# -- 空闲监测 / Idle Monitor --",
		"# -- 昼夜主题 / Day/Night Theme --",
		"# -- 设置 / Settings --",
	} {
		if !strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func configTemplateVersionMarker() string {
	return fmt.Sprintf("# config_template_version = %d", configTemplateVersion)
}

// NormalizeConfig returns a copy of cfg with invalid fields replaced by defaults.
func NormalizeConfig(cfg Config) Config {
	d := DefaultConfig()
	if cfg.Language != "auto" && cfg.Language != "en" && cfg.Language != "zh-CN" {
		cfg.Language = d.Language
	}
	if cfg.IdleTimeoutMinutes < 0 || cfg.IdleTimeoutMinutes > 7*24*60 {
		cfg.IdleTimeoutMinutes = d.IdleTimeoutMinutes
	}
	if !ValidIdleAction(cfg.IdleAction) {
		cfg.IdleAction = d.IdleAction
	}
	if cfg.IdleWarningSeconds < 0 || cfg.IdleWarningSeconds > 3600 {
		cfg.IdleWarningSeconds = d.IdleWarningSeconds
	}
	if cfg.NoSleepBatteryThreshold < 0 || cfg.NoSleepBatteryThreshold > 100 {
		cfg.NoSleepBatteryThreshold = d.NoSleepBatteryThreshold
	}
	if cfg.ThemeMode != "fixed" && cfg.ThemeMode != "sunrise" {
		cfg.ThemeMode = d.ThemeMode
	}
	if _, err := time.Parse("15:04", cfg.ThemeLightTime); err != nil {
		cfg.ThemeLightTime = d.ThemeLightTime
	}
	if _, err := time.Parse("15:04", cfg.ThemeDarkTime); err != nil {
		cfg.ThemeDarkTime = d.ThemeDarkTime
	}
	if !finiteCoordinate(cfg.ThemeLatitude) || cfg.ThemeLatitude < -90 || cfg.ThemeLatitude > 90 {
		cfg.ThemeLatitude = d.ThemeLatitude
	}
	if !finiteCoordinate(cfg.ThemeLongitude) || cfg.ThemeLongitude < -180 || cfg.ThemeLongitude > 180 {
		cfg.ThemeLongitude = d.ThemeLongitude
	}
	return cfg
}

// Validate checks values that can otherwise lead to unsafe or surprising
// runtime behavior.
func (cfg Config) Validate() error {
	switch cfg.Language {
	case "auto", "en", "zh-CN":
	default:
		return fmt.Errorf("language must be auto, en, or zh-CN")
	}
	if !ValidIdleAction(cfg.IdleAction) {
		return fmt.Errorf("invalid idle_action %q", cfg.IdleAction)
	}
	if cfg.IdleTimeoutMinutes < 0 || cfg.IdleTimeoutMinutes > 7*24*60 {
		return fmt.Errorf("idle_timeout_minutes must be between 0 and 10080")
	}
	if cfg.IdleWarningSeconds < 0 || cfg.IdleWarningSeconds > 3600 {
		return fmt.Errorf("idle_warning_seconds must be between 0 and 3600")
	}
	if cfg.NoSleepBatteryThreshold < 0 || cfg.NoSleepBatteryThreshold > 100 {
		return fmt.Errorf("nosleep_battery_threshold must be between 0 and 100")
	}
	if cfg.ThemeMode != "fixed" && cfg.ThemeMode != "sunrise" {
		return fmt.Errorf("theme_mode must be fixed or sunrise")
	}
	if _, err := time.Parse("15:04", cfg.ThemeLightTime); err != nil {
		return fmt.Errorf("invalid theme_light_time: %w", err)
	}
	if _, err := time.Parse("15:04", cfg.ThemeDarkTime); err != nil {
		return fmt.Errorf("invalid theme_dark_time: %w", err)
	}
	if !finiteCoordinate(cfg.ThemeLatitude) || cfg.ThemeLatitude < -90 || cfg.ThemeLatitude > 90 {
		return fmt.Errorf("theme_latitude must be between -90 and 90")
	}
	if !finiteCoordinate(cfg.ThemeLongitude) || cfg.ThemeLongitude < -180 || cfg.ThemeLongitude > 180 {
		return fmt.Errorf("theme_longitude must be between -180 and 180")
	}
	return nil
}

func finiteCoordinate(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// Save atomically writes the configuration to disk.
func Save(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	p, err := Path()
	if err != nil {
		return err
	}
	return saveTo(p, cfg)
}

func saveTo(p string, cfg Config) error {
	f, err := os.CreateTemp(filepath.Dir(p), ".IdleTrigger-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config file: %w", err)
	}
	tmpPath := f.Name()
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(tmpPath)
		}
	}()

	if _, err := f.WriteString(renderAnnotatedTOML(cfg)); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync config: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(tmpPath, p); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	ok = true
	return nil
}

func renderAnnotatedTOML(cfg Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", configTemplateVersionMarker())
	fmt.Fprintf(&b, "# %s\n", i18n.T(cfg.Language, "config_header"))
	fmt.Fprintf(&b, "# %s\n\n", i18n.T(cfg.Language, "config_edit_hint"))

	b.WriteString("# -- 保持唤醒 / Stay Awake --\n")
	b.WriteString("# 手动启用保持唤醒，阻止系统自动睡眠 / Manually keep the system awake and prevent automatic sleep\n")
	fmt.Fprintf(&b, "nosleep_enabled = %t\n", cfg.NoSleepEnabled)
	b.WriteString("# 保持唤醒时同步保持屏幕常亮 / Keep the display on while Stay Awake is active\n")
	fmt.Fprintf(&b, "keep_screen_on = %t\n", cfg.KeepScreenOn)
	b.WriteString("# 电池供电时仍允许保持唤醒 / Allow Stay Awake while running on battery\n")
	fmt.Fprintf(&b, "nosleep_on_battery = %t\n", cfg.NoSleepOnBattery)
	b.WriteString("# 电池电量低于此百分比时强制关闭保持唤醒 / Force-disable Stay Awake below this battery percentage\n")
	fmt.Fprintf(&b, "nosleep_battery_threshold = %d\n", cfg.NoSleepBatteryThreshold)
	b.WriteString("# 保持唤醒已开启时，仅在下方任一 exe 运行时保活；没有匹配进程时不保活，也不会单独启用保持唤醒 / When Stay Awake is enabled, keep awake only while any listed exe is running; no match means no keep-awake request, and this does not enable Stay Awake by itself\n")
	fmt.Fprintf(&b, "process_watch_enabled = %t\n", cfg.ProcessWatchEnabled)
	b.WriteString("# 适用进程的 .exe 文件名，不区分大小写；空列表是正常状态，表示不按进程限制保持唤醒，也不影响空闲监测 / Applicable .exe names, case-insensitive; an empty list is valid, means Stay Awake is not process-limited, and does not affect idle monitoring\n")
	fmt.Fprintf(&b, "process_watch_list = %s\n\n", tomlStringList(cfg.ProcessWatchList))

	b.WriteString("# -- 空闲监测 / Idle Monitor --\n")
	b.WriteString("# 空闲时长：无键鼠操作多少分钟后触发动作，设为 0 禁用 / Idle time in minutes before triggering after no keyboard or mouse input, 0 = disabled\n")
	fmt.Fprintf(&b, "idle_timeout_minutes = %d\n", cfg.IdleTimeoutMinutes)
	b.WriteString("# 达到空闲时长后执行的动作 / Action to run after the idle time is reached: \"sleep\", \"hibernate\", \"shutdown\", \"lock\"\n")
	fmt.Fprintf(&b, "idle_action = %s\n", tomlString(string(cfg.IdleAction)))
	b.WriteString("# 触发前多少秒显示不抢焦点的应用内提醒；键鼠操作或关闭提醒会取消本次动作，设为 0 静默执行 / Seconds before trigger to show a non-activating in-app reminder; keyboard/mouse input or closing it cancels this action, 0 = silent\n")
	fmt.Fprintf(&b, "idle_warning_seconds = %d\n", cfg.IdleWarningSeconds)
	b.WriteString("# 增强空闲监测：适合系统睡眠也被固定间隔空闲刷新干扰的机器；默认关闭，普通键鼠操作仍会重置计时 / Enhanced idle monitoring for machines where system sleep is disturbed by fixed-interval idle refreshes; off by default, and normal keyboard or mouse input still resets idle time\n")
	fmt.Fprintf(&b, "idle_enhanced_monitor = %t\n\n", cfg.IdleEnhancedMonitor)

	b.WriteString("# -- 昼夜主题 / Day/Night Theme --\n")
	b.WriteString("# 启用按时间自动切换 Windows 深浅色 / Automatically switch Windows light/dark theme by schedule\n")
	fmt.Fprintf(&b, "theme_switch_enabled = %t\n", cfg.ThemeSwitchEnabled)
	b.WriteString("# 切换模式：\"fixed\" 使用下方固定时间；\"sunrise\" 根据日出日落计算 / Mode: \"fixed\" uses times below; \"sunrise\" calculates sunrise/sunset\n")
	fmt.Fprintf(&b, "theme_mode = %s\n", tomlString(cfg.ThemeMode))
	b.WriteString("# 浅色开始时间，HH:MM；fixed 模式使用，日出日落不可用时也作为兜底 / Light theme start time, HH:MM; used by fixed mode and as fallback when sunrise/sunset is unavailable\n")
	fmt.Fprintf(&b, "theme_light_time = %s\n", tomlString(cfg.ThemeLightTime))
	b.WriteString("# 深色开始时间，HH:MM；fixed 模式使用，日出日落不可用时也作为兜底 / Dark theme start time, HH:MM; used by fixed mode and as fallback when sunrise/sunset is unavailable\n")
	fmt.Fprintf(&b, "theme_dark_time = %s\n", tomlString(cfg.ThemeDarkTime))
	b.WriteString("# 日出日落计算纬度，范围 -90 到 90；经纬度都为 0 时优先看下方 IP 定位开关，未开启或失败则按时区、UTC 偏移和默认位置依次回退 / Latitude for sunrise mode, -90..90; when both lat/lon are 0, the IP location option below is used first; otherwise falls back to timezone, UTC offset, then default location\n")
	fmt.Fprintf(&b, "theme_latitude = %s\n", tomlFloat(cfg.ThemeLatitude))
	b.WriteString("# 日出日落计算经度，范围 -180 到 180；经纬度都为 0 时优先看下方 IP 定位开关，未开启或失败则按时区、UTC 偏移和默认位置依次回退 / Longitude for sunrise mode, -180..180; when both lat/lon are 0, the IP location option below is used first; otherwise falls back to timezone, UTC offset, then default location\n")
	fmt.Fprintf(&b, "theme_longitude = %s\n", tomlFloat(cfg.ThemeLongitude))
	b.WriteString("# 经纬度都为 0 时，通过 https://ipwho.is/ 估算公网 IP 所在城市；成功结果仅内存缓存 24 小时，每次启动或手动重新开启定位时，首次失败后会在 30 分钟后补试一次；关闭或失败时按时区、UTC 偏移和默认位置依次回退 / When coordinates are 0, estimate city via https://ipwho.is/; successful results are cached in memory for 24 hours, and one retry is made 30 minutes after the first failure on app start or explicit re-enable; falls back to timezone, UTC offset, then default location when off or failed\n")
	fmt.Fprintf(&b, "theme_ip_location_enabled = %t\n", cfg.ThemeIPLocationEnabled)
	b.WriteString("# 电池供电时自动切换深色，接入电源后按当前计划恢复 / Switch to dark on battery, then restore by schedule on AC power\n")
	fmt.Fprintf(&b, "theme_dark_on_battery = %t\n", cfg.ThemeDarkOnBattery)
	b.WriteString("# 全屏应用或游戏运行时暂不自动切换主题 / Pause automatic theme switching during fullscreen apps/games\n")
	fmt.Fprintf(&b, "theme_skip_fullscreen = %t\n\n", cfg.ThemeSkipFullscreen)

	b.WriteString("# -- 设置 / Settings --\n")
	b.WriteString("# 启用全局热键：Win+Shift+S 睡眠，Win+Shift+L 锁定，Win+Shift+N 切换保持唤醒 / Enable global hotkeys: Win+Shift+S sleep, Win+Shift+L lock, Win+Shift+N toggle Stay Awake\n")
	fmt.Fprintf(&b, "hotkeys_enabled = %t\n", cfg.HotkeysEnabled)
	b.WriteString("# 将调试日志写入 EXE 同目录的 IdleTrigger.log；每行带启动会话标识 / Write debug logs to IdleTrigger.log next to the EXE; each line includes a startup session ID\n")
	fmt.Fprintf(&b, "logging_enabled = %t\n", cfg.LoggingEnabled)
	b.WriteString("# 界面语言：\"auto\" 跟随系统，\"en\" 英文，\"zh-CN\" 简体中文 / UI language: \"auto\" follows OS, \"en\" English, \"zh-CN\" Simplified Chinese\n")
	fmt.Fprintf(&b, "language = %s\n", tomlString(cfg.Language))

	return b.String()
}

func tomlString(value string) string {
	return strconv.Quote(value)
}

func tomlStringList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, tomlString(value))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func tomlFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
