// Package config reads and writes IdleTrigger's TOML configuration file.
// The config file (IdleTrigger.toml) lives next to the executable and is
// plain-text TOML — users can edit it directly.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		ThemeMode:               "sunrise",
		ThemeLatitude:           0,
		ThemeLongitude:          0,
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
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

// Validate checks values that can otherwise lead to unsafe or surprising
// runtime behavior.
func (cfg Config) Validate() error {
	switch cfg.Language {
	case "auto", "en", "zh-CN":
	default:
		return fmt.Errorf("language must be auto, en, or zh-CN")
	}
	switch cfg.IdleAction {
	case ActionSleep, ActionHibernate, ActionShutdown, ActionLock:
	default:
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
	if cfg.ThemeLatitude < -90 || cfg.ThemeLatitude > 90 {
		return fmt.Errorf("theme_latitude must be between -90 and 90")
	}
	if cfg.ThemeLongitude < -180 || cfg.ThemeLongitude > 180 {
		return fmt.Errorf("theme_longitude must be between -180 and 180")
	}
	return nil
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

	if _, err := fmt.Fprintf(f, "# %s\n", i18n.T(cfg.Language, "config_header")); err != nil {
		return fmt.Errorf("write config header: %w", err)
	}
	if _, err := fmt.Fprintf(f, "# %s\n\n", i18n.T(cfg.Language, "config_edit_hint")); err != nil {
		return fmt.Errorf("write config header: %w", err)
	}

	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
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
