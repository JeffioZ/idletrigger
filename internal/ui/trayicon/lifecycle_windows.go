package trayicon

import "unsafe"

func registerSystray() {
	if err := wt.initInstance(); err != nil {
		reportError("Unable to init instance: %v", err)
		return
	}

	if err := wt.createMenu(); err != nil {
		reportError("Unable to create menu: %v", err)
		return
	}

	systrayReady()
}

func nativeLoop() {
	// Main message pump.
	m := &message{}
	for {
		ret, _, err := pGetMessage.Call(uintptr(unsafe.Pointer(m)), 0, 0, 0)

		// If the function retrieves a message other than WM_QUIT, the return value is nonzero.
		// If the function retrieves the WM_QUIT message, the return value is zero.
		// If there is an error, the return value is -1
		// https://msdn.microsoft.com/en-us/library/windows/desktop/ms644936(v=vs.85).aspx
		switch int32(ret) {
		case -1:
			reportError("Error at message loop: %v", err)
			return
		case 0:
			return
		default:
			if dispatchTabNavigation(m) {
				continue
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(m)))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(m)))
		}
	}
}

func quit() {
	const WM_CLOSE = 0x0010

	pPostMessage.Call(
		uintptr(wt.window),
		WM_CLOSE,
		0,
		0,
	)
}

// SetIconResource sets the tray icon from an RT_GROUP_ICON resource embedded
// in the current executable.
func SetIconResource(resourceID uint16) {
	if err := wt.setIcon(resourceID); err != nil {
		reportError("Unable to set icon: %v", err)
		return
	}
}

// SetTitle sets the systray title, only available on Mac and Linux.
func SetTitle(title string) {
	// do nothing
}

func (item *MenuItem) parentId() uint32 {
	if item.parent != nil {
		return uint32(item.parent.id)
	}
	return 0
}

// SetIconResource sets a menu item's icon from an RT_GROUP_ICON resource
// embedded in the current executable.
func (item *MenuItem) SetIconResource(resourceID uint16) {
	h, err := wt.loadIconResource(resourceID)
	if err != nil {
		reportError("Unable to load menu icon resource: %v", err)
		return
	}

	h, err = wt.iconToBitmap(h)
	if err != nil {
		reportError("Unable to convert icon to bitmap: %v", err)
		return
	}
	wt.muMenuItemIcons.Lock()
	wt.menuItemIcons[uint32(item.id)] = h
	wt.muMenuItemIcons.Unlock()

	err = wt.addOrUpdateMenuItem(uint32(item.id), item.parentId(), item.title, item.disabled, item.checked)
	if err != nil {
		reportError("Unable to addOrUpdateMenuItem: %v", err)
		return
	}
}

// SetTooltip sets the systray tooltip to display on mouse hover of the tray icon,
// only available on Mac and Windows.
func SetTooltip(tooltip string) {
	if err := wt.setTooltip(tooltip); err != nil {
		reportError("Unable to set tooltip: %v", err)
		return
	}
}

func addOrUpdateMenuItem(item *MenuItem) {
	err := wt.addOrUpdateMenuItem(uint32(item.id), item.parentId(), item.title, item.disabled, item.checked)
	if err != nil {
		reportError("Unable to addOrUpdateMenuItem: %v", err)
		return
	}
}

func addSeparator(id uint32) {
	err := wt.addSeparatorMenuItem(id, 0)
	if err != nil {
		reportError("Unable to addSeparator: %v", err)
		return
	}
}

func hideMenuItem(item *MenuItem) {
	err := wt.hideMenuItem(uint32(item.id), item.parentId())
	if err != nil {
		reportError("Unable to hideMenuItem: %v", err)
		return
	}
}

func showMenuItem(item *MenuItem) {
	addOrUpdateMenuItem(item)
}
