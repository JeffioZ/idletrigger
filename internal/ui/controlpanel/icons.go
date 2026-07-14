package controlpanel

import (
	"golang.org/x/sys/windows"

	mylog "github.com/JeffioZ/idletrigger/internal/logging"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/resourceid"
)

const (
	appIconResourceID       = resourceid.AppIconID       // Main executable icon; shared class fallback only.
	trayDarkIconResourceID  = resourceid.TrayDarkIconID  // Dark mark for a light title bar.
	trayLightIconResourceID = resourceid.TrayLightIconID // Light mark for a dark title bar.
)

// classFallbackIcon is loaded through LoadIconW, whose resource handle is
// shared by the module and must not be destroyed. Every real panel instance
// immediately replaces it with the theme-specific, owned WM_SETICON handles.
func classFallbackIcon(module uintptr) windows.Handle {
	if module == 0 {
		return 0
	}
	icon, _, _ := pLoadIcon.Call(module, appIconResourceID)
	return windows.Handle(icon)
}

func windowIconResourceID(dark bool) uintptr {
	if dark {
		return trayLightIconResourceID
	}
	return trayDarkIconResourceID
}

func windowIcon(dark bool, size int) windows.Handle {
	module, _, _ := pGetModuleHandle.Call(0)
	if module == 0 {
		return 0
	}
	const imageIcon = 1
	icon, _, _ := pLoadImage.Call(module, windowIconResourceID(dark), imageIcon, uintptr(size), uintptr(size), 0)
	return windows.Handle(icon)
}

func shouldReloadWindowIcons(initialized, currentDark, requestedDark, force bool) bool {
	return !initialized || force || currentDark != requestedDark
}

func (p *panel) setWindowIcons(dark, force bool) {
	if p.hwnd == 0 || !shouldReloadWindowIcons(p.iconsInitialized, p.iconThemeDark, dark, force) {
		return
	}
	largeIcon := windowIcon(dark, p.sc(p.metrics.style.Control.IconLarge))
	smallIcon := windowIcon(dark, p.sc(p.metrics.style.Control.IconSmall))
	if largeIcon == 0 || smallIcon == 0 {
		if largeIcon != 0 {
			pDestroyIcon.Call(uintptr(largeIcon))
		}
		if smallIcon != 0 {
			pDestroyIcon.Call(uintptr(smallIcon))
		}
		mylog.Info("UI icon: surface=popup load failed theme_dark=%v dpi=%d", dark, int(p.metrics.scale*96+0.5))
		return
	}
	oldLargeIcon, oldSmallIcon := p.largeIcon, p.smallIcon
	p.largeIcon, p.smallIcon = largeIcon, smallIcon
	p.iconThemeDark, p.iconsInitialized = dark, true
	const (
		iconSmall = 0
		iconBig   = 1
	)
	pSendMessage.Call(uintptr(p.hwnd), wmSetIcon, iconBig, uintptr(p.largeIcon))
	pSendMessage.Call(uintptr(p.hwnd), wmSetIcon, iconSmall, uintptr(p.smallIcon))
	for _, icon := range []windows.Handle{oldLargeIcon, oldSmallIcon} {
		if icon != 0 {
			pDestroyIcon.Call(uintptr(icon))
		}
	}
}
