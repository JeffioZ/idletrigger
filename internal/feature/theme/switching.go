package theme

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	themeSwitchUser32              = windows.NewLazySystemDLL("user32.dll")
	pUpdatePerUserSystemParameters = themeSwitchUser32.NewProc("UpdatePerUserSystemParameters")
	pSendNotifyMessage             = themeSwitchUser32.NewProc("SendNotifyMessageW")
	pSendMessageTimeout            = themeSwitchUser32.NewProc("SendMessageTimeoutW")
)

const regPath = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`

// DetectSupport verifies that the current Windows profile exposes writable
// light/dark Personalize settings. It deliberately does not change either
// value, so capability detection cannot cause a visible theme switch.
func DetectSupport() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Windows Personalize settings: %w", err)
	}
	defer k.Close()
	for _, name := range []string{"AppsUseLightTheme", "SystemUsesLightTheme"} {
		value, valueType, err := k.GetIntegerValue(name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if valueType != registry.DWORD || value > 1 {
			return fmt.Errorf("%s is not a valid Windows light/dark setting", name)
		}
	}
	return nil
}

// Mode is the target theme mode.
type Mode int

const (
	ModeLight Mode = iota
	ModeDark
)

// Switch sets the Windows app and system theme.
func Switch(mode Mode) error {
	var val uint32
	if mode == ModeLight {
		val = 1
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetDWordValue("AppsUseLightTheme", val); err != nil {
		return fmt.Errorf("set AppsUseLightTheme: %w", err)
	}
	if err := k.SetDWordValue("SystemUsesLightTheme", val); err != nil {
		return fmt.Errorf("set SystemUsesLightTheme: %w", err)
	}
	for _, name := range []string{"AppsUseLightTheme", "SystemUsesLightTheme"} {
		actual, valueType, err := k.GetIntegerValue(name)
		if err != nil {
			return fmt.Errorf("verify %s: %w", name, err)
		}
		if valueType != registry.DWORD || uint32(actual) != val {
			return fmt.Errorf("verify %s: Windows did not retain the requested value", name)
		}
	}

	notifyThemeChanged()
	return nil
}

// Refresh asks Windows shell surfaces to re-read the current theme settings.
func Refresh() error {
	notifyThemeChanged()
	return nil
}

func notifyThemeChanged() {
	const (
		hwndBroadcast    = 0xFFFF
		wmThemeChanged   = 0x031A
		wmSysColorChange = 0x0015
		wmSettingChange  = 0x001A
		smtoAbortIfHung  = 0x0002
	)

	pUpdatePerUserSystemParameters.Call(0, 1)

	// Notify every top-level window, including Explorer's taskbars on each
	// display. Windows components do not all listen for the same token, so send
	// the common theme and color notifications without blocking on hung apps.
	for _, token := range []string{"ImmersiveColorSet", "WindowsThemeElement", "Policy"} {
		ptr, _ := windows.UTF16PtrFromString(token)
		lp := uintptr(unsafe.Pointer(ptr))
		pSendNotifyMessage.Call(hwndBroadcast, wmSettingChange, 0, lp)
		pSendMessageTimeout.Call(hwndBroadcast, wmSettingChange, 0, lp, smtoAbortIfHung, 250, 0)
	}
	pSendNotifyMessage.Call(hwndBroadcast, wmSysColorChange, 0, 0)
	pSendMessageTimeout.Call(hwndBroadcast, wmSysColorChange, 0, 0, smtoAbortIfHung, 250, 0)
	pSendNotifyMessage.Call(hwndBroadcast, wmThemeChanged, 0, 0)
	pSendMessageTimeout.Call(hwndBroadcast, wmThemeChanged, 0, 0, smtoAbortIfHung, 250, 0)
}

// Current returns the current theme mode.
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
