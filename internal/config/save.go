package config

import (
	"fmt"
	"os"
	"path/filepath"
)

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
