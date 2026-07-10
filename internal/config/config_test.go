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
		!strings.Contains(text, "idle_action = \"sleep\"") {
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
