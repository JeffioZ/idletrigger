package systray

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestMissingMenuItemUsesErrorHandler(t *testing.T) {
	var message string
	SetErrorHandler(func(format string, args ...interface{}) {
		message = fmt.Sprintf(format, args...)
	})
	t.Cleanup(func() { SetErrorHandler(nil) })

	systrayMenuItemSelected(^uint32(0))
	if !strings.Contains(message, "No menu item with ID") {
		t.Fatalf("unexpected error message: %q", message)
	}
}

func TestTabNavigationMessageScope(t *testing.T) {
	dialog := windows.Handle(100)
	child := windows.Handle(101)
	tests := []struct {
		name    string
		message *message
		isChild bool
		want    bool
	}{
		{name: "popup tab", message: &message{WindowHandle: dialog, Message: wmKeyDown, Wparam: vkTab}, want: true},
		{name: "child tab", message: &message{WindowHandle: child, Message: wmKeyDown, Wparam: vkTab}, isChild: true, want: true},
		{name: "unrelated window", message: &message{WindowHandle: child, Message: wmKeyDown, Wparam: vkTab}, want: false},
		{name: "enter remains normal", message: &message{WindowHandle: dialog, Message: wmKeyDown, Wparam: 0x0D}, want: false},
		{name: "tab character remains normal", message: &message{WindowHandle: dialog, Message: 0x0102, Wparam: vkTab}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTabNavigationMessage(tt.message, dialog, tt.isChild); got != tt.want {
				t.Fatalf("isTabNavigationMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestThemeChangeMessageScope(t *testing.T) {
	for _, message := range []uint32{wmSettingChange, wmSysColorChange, wmThemeChanged} {
		if !isThemeChangeMessage(message) {
			t.Fatalf("theme message %#x was not recognized", message)
		}
	}
	if isThemeChangeMessage(wmKeyDown) {
		t.Fatal("keyboard messages must not trigger a theme refresh")
	}
}
