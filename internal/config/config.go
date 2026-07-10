// Package config reads and writes IdleTrigger's TOML configuration file.
// The config file (IdleTrigger.toml) lives next to the executable and is
// plain-text TOML — users can edit it directly.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Action is a system power action.
type Action string

const (
	ActionSleep     Action = "sleep"
	ActionHibernate Action = "hibernate"
	ActionShutdown  Action = "shutdown"
	ActionLock      Action = "lock"
)

// Config holds all user-configurable settings.
type Config struct {
	// Language for UI strings: "auto" (follow OS), "en", "zh-CN".
	Language string `toml:"language"`

	// IdleTimeoutMinutes is how many minutes of inactivity before the idle
	// action fires. 0 disables the idle monitor.
	IdleTimeoutMinutes int `toml:"idle_timeout_minutes"`

	// IdleAction is the system action to trigger when idle timeout expires.
	IdleAction Action `toml:"idle_action"`

	// IdleWarningSeconds — show a notification this many seconds before the
	// idle action fires. 0 disables the warning.  Default: 30.
	IdleWarningSeconds int `toml:"idle_warning_seconds"`

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

	// ProcessWatchEnabled turns on automatic NoSleep when watched apps run.
	ProcessWatchEnabled bool `toml:"process_watch_enabled"`

	// ProcessWatchList is a list of case-insensitive .exe names.  When any
	// are running NoSleep auto-enables (if ProcessWatchEnabled is true).
	ProcessWatchList []string `toml:"process_watch_list"`

	// LoggingEnabled writes debug logs to IdleTrigger.log.
	LoggingEnabled bool `toml:"logging_enabled"`

	// ThemeSwitchEnabled auto-switches Windows theme at scheduled times.
	ThemeSwitchEnabled bool `toml:"theme_switch_enabled"`

	// ThemeLightTime is when to switch to light mode (HH:MM).
	ThemeLightTime string `toml:"theme_light_time"`

	// ThemeDarkTime is when to switch to dark mode (HH:MM).
	ThemeDarkTime string `toml:"theme_dark_time"`

	// ThemeMode: "fixed" (use times above) or "sunrise" (calculate from lat/lon).
	ThemeMode string `toml:"theme_mode"`

	// ThemeLatitude / ThemeLongitude for sunrise calculation.
	ThemeLatitude  float64 `toml:"theme_latitude"`
	ThemeLongitude float64 `toml:"theme_longitude"`

	// StartMinimized: when true the tray icon starts without showing any window.
	StartMinimized bool `toml:"start_minimized"`

	// AutostartEnabled: when true a registry Run entry starts IdleTrigger on boot.
	AutostartEnabled bool `toml:"autostart_enabled"`
}

// DefaultConfig returns the factory-default configuration.
func DefaultConfig() Config {
	return Config{
		Language:                "auto",
		IdleTimeoutMinutes:      30,
		IdleAction:              ActionSleep,
		IdleWarningSeconds:      30,
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
		ThemeMode:               "fixed",
		ThemeLatitude:           39.9,
		ThemeLongitude:          116.4,
		StartMinimized:          true,
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
	cfg := DefaultConfig()
	p, err := Path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// First run — write defaults and return them.
			if saveErr := Save(cfg); saveErr != nil {
				return cfg, saveErr
			}
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Save writes the configuration to disk.
func Save(cfg Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	// Write a human-readable header.
	f.WriteString("# IdleTrigger Configuration\n")
	f.WriteString("# Edit this file directly and restart IdleTrigger to apply changes.\n\n")

	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return nil
}
