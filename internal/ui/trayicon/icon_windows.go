package trayicon

import (
	"golang.org/x/sys/windows"
	"strings"
	"unsafe"
)

// Loads an icon resource embedded in the executable and shows it in the tray.
// Shell_NotifyIcon: https://msdn.microsoft.com/en-us/library/windows/desktop/bb762159(v=vs.85).aspx
func (t *winTray) setIcon(resourceID uint16) error {
	const NIF_ICON = 0x00000002
	t.muIconLifecycle.Lock()
	defer t.muIconLifecycle.Unlock()

	h, err := t.loadIconResource(resourceID)
	if err != nil {
		return err
	}

	t.muNID.Lock()
	defer t.muNID.Unlock()
	t.nid.Icon = h
	t.nid.Flags |= NIF_ICON
	t.nid.Size = uint32(unsafe.Sizeof(*t.nid))

	return t.nid.modify()
}

// releaseLoadedIcons destroys only the HICON values created by LoadImageW in
// loadIconResource. It runs after NIM_DELETE (or destruction of the owner window),
// when Shell_NotifyIcon can no longer use the notification icon handle.
func (t *winTray) releaseLoadedIcons() {
	for _, icon := range t.takeLoadedIconsForRelease() {
		pDestroyIcon.Call(uintptr(icon))
	}
}

// takeLoadedIconsForRelease transfers the owned handles exactly once. Clearing
// the map before DestroyIcon prevents both a second release and later reuse of
// an invalid handle during shutdown.
func (t *winTray) takeLoadedIconsForRelease() []windows.Handle {
	t.muLoadedImages.Lock()
	defer t.muLoadedImages.Unlock()
	if t.iconsReleased {
		return nil
	}
	t.iconsReleased = true
	icons := make([]windows.Handle, 0, len(t.loadedImages))
	for _, icon := range t.loadedImages {
		if icon != 0 {
			icons = append(icons, icon)
		}
	}
	t.loadedImages = nil
	return icons
}

// removeNotificationIcon releases the Shell registration before destroying
// application-owned HICON values. WM_ENDSESSION can precede WM_DESTROY, so this
// operation is intentionally idempotent.
func (t *winTray) removeNotificationIcon() {
	t.muIconLifecycle.Lock()
	defer t.muIconLifecycle.Unlock()
	t.muNID.Lock()
	nid := t.nid
	t.nid = nil
	t.muNID.Unlock()
	if nid != nil {
		_ = nid.delete()
	}
	t.releaseLoadedIcons()
}

// Sets tooltip on icon.
// Shell_NotifyIcon: https://msdn.microsoft.com/en-us/library/windows/desktop/bb762159(v=vs.85).aspx
func (t *winTray) setTooltip(src string) error {
	const NIF_TIP = 0x00000004
	src = fitTooltip(src, len(t.nid.Tip)-1)
	b, err := windows.UTF16FromString(src)
	if err != nil {
		return err
	}

	t.muNID.Lock()
	defer t.muNID.Unlock()
	for i := range t.nid.Tip {
		t.nid.Tip[i] = 0
	}
	if len(b) > len(t.nid.Tip) {
		b = b[:len(t.nid.Tip)]
	}
	if len(b) > 0 {
		copy(t.nid.Tip[:], b[:len(b)-1])
	}
	t.nid.Flags |= NIF_TIP
	t.nid.Size = uint32(unsafe.Sizeof(*t.nid))

	return t.nid.modify()
}

func fitTooltip(src string, maxUTF16 int) string {
	src = strings.TrimSpace(src)
	if maxUTF16 <= 0 {
		return ""
	}
	units := 0
	var out []rune
	for _, r := range src {
		width := 1
		if r > 0xFFFF {
			width = 2
		}
		if units+width > maxUTF16 {
			break
		}
		out = append(out, r)
		units += width
	}
	return trimTooltipSuffix(string(out))
}

func trimTooltipSuffix(src string) string {
	return strings.TrimRight(strings.TrimSpace(src), " \t\r\n·:：/-")
}

var wt winTray

const wmRunUITask = 0x8001
