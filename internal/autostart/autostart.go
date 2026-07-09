// Package autostart manages the Windows registry Run key so the user
// can enable or disable IdleTrigger auto-start on login.
package autostart

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey  = `Software\Microsoft\Windows\CurrentVersion\Run`
	valName = "IdleTrigger"
)

// IsEnabled returns true when the auto-start registry entry is present.
func IsEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}
	defer k.Close()

	_, _, err = k.GetStringValue(valName)
	if err == registry.ErrNotExist {
		return false, nil
	}
	return err == nil, err
}

// Enable writes the auto-start registry entry pointing to the current
// executable with a "--minimized" argument so it starts to tray.
func Enable() error {
	exe, err := exePath()
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer k.Close()

	cmd := fmt.Sprintf(`"%s" --minimized`, exe)
	return k.SetStringValue(valName, cmd)
}

// Disable removes the auto-start registry entry.
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		// If the key doesn't exist there's nothing to delete — success.
		if err == registry.ErrNotExist {
			return nil
		}
		return fmt.Errorf("open Run key: %w", err)
	}
	defer k.Close()

	err = k.DeleteValue(valName)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}

func exePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Clean(p), nil
}
