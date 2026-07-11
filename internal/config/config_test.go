package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestDefaultConfigValid(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("default config must be valid: %v", err)
	}
}

func TestExampleConfigParsesAndValidates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "IdleTrigger.example.toml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse example config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate example config: %v", err)
	}
	text := string(data)
	assertConfigFieldsPresent(t, text)
	assertConfigOrder(t, text)
}

func TestValidateRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"action", func(c *Config) { c.IdleAction = "format" }},
		{"negative timeout", func(c *Config) { c.IdleTimeoutMinutes = -1 }},
		{"battery threshold", func(c *Config) { c.NoSleepBatteryThreshold = 101 }},
		{"theme time", func(c *Config) { c.ThemeDarkTime = "not-a-time" }},
		{"latitude", func(c *Config) { c.ThemeLatitude = 91 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSaveToAtomicallyReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "IdleTrigger.toml")
	if err := os.WriteFile(path, []byte("old content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveTo(path, DefaultConfig()); err != nil {
		t.Fatalf("saveTo: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "old content") ||
		strings.Contains(text, "autostart_enabled") ||
		!strings.Contains(text, configTemplateVersionMarker()) ||
		!strings.Contains(text, "idle_action = \"sleep\"") ||
		!strings.Contains(text, "# -- 空闲监测 / Idle Monitor --") {
		t.Fatalf("unexpected saved config: %s", text)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".IdleTrigger-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestSaveToLocalizesHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "IdleTrigger.toml")
	cfg := DefaultConfig()
	cfg.Language = "zh-CN"
	if err := saveTo(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "# IdleTrigger 配置") ||
		!strings.Contains(text, "运行 config:reload") {
		t.Fatalf("localized config header missing: %s", text)
	}
}

func TestSaveToWritesAnnotatedConfigThatParses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "IdleTrigger.toml")
	cfg := DefaultConfig()
	cfg.Language = "zh-CN"
	cfg.ProcessWatchList = []string{"chrome.exe", "powerpnt.exe"}
	cfg.ThemeLatitude = 31.2304
	cfg.ThemeLongitude = 121.4737
	if err := saveTo(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		configTemplateVersionMarker(),
		"# -- 保持唤醒 / Stay Awake --",
		"# -- 空闲监测 / Idle Monitor --",
		"# -- 昼夜主题 / Day/Night Theme --",
		"# -- 设置 / Settings --",
		"process_watch_list = [\"chrome.exe\", \"powerpnt.exe\"]",
		"theme_latitude = 31.2304",
		"theme_longitude = 121.4737",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("saved config missing %q:\n%s", want, text)
		}
	}
	assertConfigFieldsPresent(t, text)
	assertConfigOrder(t, text)

	parsed := DefaultConfig()
	if err := toml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse saved config: %v", err)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("validate saved config: %v", err)
	}
	if len(parsed.ProcessWatchList) != 2 || parsed.ProcessWatchList[0] != "chrome.exe" || parsed.ThemeLatitude != 31.2304 {
		t.Fatalf("parsed config mismatch: %+v", parsed)
	}
}

func TestLoadFromRefreshesExistingPlainConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "IdleTrigger.toml")
	plain := strings.Join([]string{
		"language = \"zh-CN\"",
		"idle_timeout_minutes = 5",
		"idle_action = \"lock\"",
		"nosleep_enabled = true",
		"process_watch_list = [\"obs64.exe\"]",
		"theme_mode = \"fixed\"",
		"theme_light_time = \"08:30\"",
		"theme_dark_time = \"20:45\"",
	}, "\n")
	if err := os.WriteFile(path, []byte(plain), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if cfg.Language != "zh-CN" ||
		cfg.IdleTimeoutMinutes != 5 ||
		cfg.IdleAction != ActionLock ||
		!cfg.NoSleepEnabled ||
		len(cfg.ProcessWatchList) != 1 ||
		cfg.ProcessWatchList[0] != "obs64.exe" ||
		cfg.ThemeLightTime != "08:30" ||
		cfg.ThemeDarkTime != "20:45" {
		t.Fatalf("loaded config mismatch: %+v", cfg)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	infoAfterRefresh, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	assertConfigFieldsPresent(t, text)
	assertConfigOrder(t, text)
	if needsAnnotatedTOMLRefresh(data) {
		t.Fatalf("refreshed config should not need another refresh:\n%s", text)
	}
	for _, want := range []string{
		configTemplateVersionMarker(),
		"# -- 保持唤醒 / Stay Awake --",
		"idle_timeout_minutes = 5",
		"idle_action = \"lock\"",
		"nosleep_enabled = true",
		"process_watch_list = [\"obs64.exe\"]",
		"theme_light_time = \"08:30\"",
		"theme_dark_time = \"20:45\"",
		"language = \"zh-CN\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("refreshed config missing %q:\n%s", want, text)
		}
	}

	if _, err := loadFrom(path); err != nil {
		t.Fatalf("second loadFrom: %v", err)
	}
	infoAfterSecondLoad, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(infoAfterRefresh, infoAfterSecondLoad) {
		t.Fatal("annotated config was rewritten on the second load")
	}
}

func TestLoadFromRefreshesOlderTemplateVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "IdleTrigger.toml")
	oldAnnotated := renderAnnotatedTOML(DefaultConfig())
	oldAnnotated = strings.Replace(oldAnnotated, configTemplateVersionMarker(), "# config_template_version = 1", 1)
	oldAnnotated = strings.Replace(oldAnnotated, "idle_timeout_minutes = 30", "idle_timeout_minutes = 120", 1)
	if err := os.WriteFile(path, []byte(oldAnnotated), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if cfg.IdleTimeoutMinutes != 120 {
		t.Fatalf("user value was not preserved: %+v", cfg)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, configTemplateVersionMarker()) ||
		!strings.Contains(text, "idle_timeout_minutes = 120") ||
		needsAnnotatedTOMLRefresh(data) {
		t.Fatalf("old template version was not refreshed correctly:\n%s", text)
	}
}

func assertConfigFieldsPresent(t *testing.T, text string) {
	t.Helper()
	for _, field := range []string{
		"config_template_version",
		"nosleep_enabled =",
		"keep_screen_on =",
		"nosleep_on_battery =",
		"nosleep_battery_threshold =",
		"process_watch_enabled =",
		"process_watch_list =",
		"idle_timeout_minutes =",
		"idle_action =",
		"idle_warning_seconds =",
		"theme_switch_enabled =",
		"theme_mode =",
		"theme_light_time =",
		"theme_dark_time =",
		"theme_latitude =",
		"theme_longitude =",
		"theme_dark_on_battery =",
		"theme_skip_fullscreen =",
		"hotkeys_enabled =",
		"logging_enabled =",
		"language =",
	} {
		if !strings.Contains(text, field) {
			t.Fatalf("config missing field %q:\n%s", field, text)
		}
	}
	if strings.Contains(text, "autostart_enabled") {
		t.Fatalf("runtime-only autostart field should not be saved:\n%s", text)
	}
}

func assertConfigOrder(t *testing.T, text string) {
	t.Helper()
	needles := []string{
		"# -- 保持唤醒 / Stay Awake --",
		"nosleep_enabled =",
		"process_watch_list =",
		"# -- 空闲监测 / Idle Monitor --",
		"idle_timeout_minutes =",
		"# -- 昼夜主题 / Day/Night Theme --",
		"theme_switch_enabled =",
		"# -- 设置 / Settings --",
		"hotkeys_enabled =",
		"logging_enabled =",
		"language =",
	}
	last := -1
	for _, needle := range needles {
		pos := strings.Index(text, needle)
		if pos < 0 {
			t.Fatalf("config missing %q:\n%s", needle, text)
		}
		if pos <= last {
			t.Fatalf("config order mismatch at %q:\n%s", needle, text)
		}
		last = pos
	}
}
