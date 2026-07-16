package trayicon

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

func TestNestedTabNavigationRestoresOwner(t *testing.T) {
	tabNavigation.Lock()
	tabNavigation.hwnd = 0
	tabNavigation.onNavigated = nil
	tabNavigation.stack = nil
	tabNavigation.Unlock()
	t.Cleanup(func() {
		tabNavigation.Lock()
		tabNavigation.hwnd = 0
		tabNavigation.onNavigated = nil
		tabNavigation.stack = nil
		tabNavigation.Unlock()
	})

	owner, child, warning := windows.Handle(100), windows.Handle(200), windows.Handle(300)
	ownerNavigated := 0
	SetTabNavigationWindow(owner, func() { ownerNavigated++ })
	SetTabNavigationWindow(child, nil)
	SetTabNavigationWindow(warning, nil)
	ClearTabNavigationWindow(child)
	ClearTabNavigationWindow(warning)
	if tabNavigation.hwnd != owner || len(tabNavigation.stack) != 0 {
		t.Fatalf("active=%d stack=%+v", tabNavigation.hwnd, tabNavigation.stack)
	}
	tabNavigation.onNavigated()
	if ownerNavigated != 1 {
		t.Fatal("nested dialog cleanup did not restore the owner's Tab-navigation callback")
	}
	ClearTabNavigationWindow(owner)
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

func TestTrayHostWindowCannotExposeANormalFrame(t *testing.T) {
	const (
		wsVisible = 0x10000000
		wsCaption = 0x00C00000
		wsPopup   = 0x80000000
	)
	if trayHostWindowStyle&wsPopup == 0 {
		t.Fatal("tray host must remain a top-level popup for broadcast messages")
	}
	if trayHostWindowStyle&(wsVisible|wsCaption) != 0 {
		t.Fatalf("tray host style %#x can expose a visible framed window", trayHostWindowStyle)
	}
	if trayHostWindowCoordinate > -30000 {
		t.Fatalf("tray host creation coordinate %d is not safely outside the desktop", trayHostWindowCoordinate)
	}
}

func TestTakeLoadedIconsForReleaseTransfersHandlesOnce(t *testing.T) {
	tray := &winTray{loadedImages: map[uint16]windows.Handle{
		1: 101,
		2: 202,
		3: 0,
	}}
	first := tray.takeLoadedIconsForRelease()
	if len(first) != 2 || !tray.iconsReleased || tray.loadedImages != nil {
		t.Fatalf("first release = %#v, released=%v, cache=%#v", first, tray.iconsReleased, tray.loadedImages)
	}
	seen := map[windows.Handle]bool{}
	for _, icon := range first {
		seen[icon] = true
	}
	if !seen[101] || !seen[202] {
		t.Fatalf("released handles = %#v, want 101 and 202", first)
	}
	if second := tray.takeLoadedIconsForRelease(); len(second) != 0 {
		t.Fatalf("second release = %#v, want no handles", second)
	}
}

func TestWaitForUITaskStopsDuringShutdown(t *testing.T) {
	done := make(chan struct{})
	stopped := make(chan struct{})
	close(stopped)
	if waitForUITask(done, stopped) {
		t.Fatal("shutdown should cancel a pending synchronous UI task")
	}

	close(done)
	if !waitForUITask(done, make(chan struct{})) {
		t.Fatal("completed UI task should report success")
	}
}

func TestBeginUIShutdownIsIdempotent(t *testing.T) {
	stopped := make(chan struct{})
	tray := &winTray{
		window:    123,
		uiTasks:   []func(){func() {}},
		uiStopped: stopped,
	}
	tray.beginUIShutdown()
	tray.beginUIShutdown()
	if tray.window != 0 || !tray.uiClosing || tray.uiTasks != nil {
		t.Fatalf("shutdown state: window=%v closing=%v tasks=%d", tray.window, tray.uiClosing, len(tray.uiTasks))
	}
	select {
	case <-stopped:
	default:
		t.Fatal("shutdown did not notify synchronous UI waiters")
	}
}
