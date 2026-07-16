package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
)

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
		"# -- 自动任务 / Automatic Tasks --",
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
