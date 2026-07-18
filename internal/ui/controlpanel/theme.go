package controlpanel

import (
	"github.com/JeffioZ/idletrigger/internal/ui/colors"
	"golang.org/x/sys/windows"
	"unsafe"
)

func (p *panel) refreshTheme(invalidate bool) {
	if p.themeRefreshing {
		return
	}
	p.themeRefreshing = true
	defer func() { p.themeRefreshing = false }()
	p.closeChoice(false)
	dark := p.resolveTheme()
	p.themeDark = dark
	p.setWindowIcons(dark, false)
	p.palette = colors.ForTheme(dark)
	p.rebuildBrushes(p.palette)
	p.applyFrameTheme(dark)
	p.applyTooltipTheme(dark)
	if invalidate && p.hwnd != 0 {
		pInvalidateRect.Call(uintptr(p.hwnd), 0, 1)
		for id := range p.controls {
			p.invalidate(id)
		}
	}
}

func (p *panel) applyTooltipTheme(dark bool) {
	if p.tooltip == 0 {
		return
	}
	if dark {
		if name, err := windows.UTF16PtrFromString("DarkMode_Explorer"); err == nil {
			pSetWindowTheme.Call(uintptr(p.tooltip), uintptr(unsafe.Pointer(name)), 0)
		}
	} else {
		pSetWindowTheme.Call(uintptr(p.tooltip), 0, 0)
	}
	pSendMessage.Call(uintptr(p.tooltip), ttmSetTipBkColor, uintptr(p.palette.TooltipBackground), 0)
	pSendMessage.Call(uintptr(p.tooltip), ttmSetTipTextColor, uintptr(p.palette.TooltipText), 0)
}

func (p *panel) applyFrameTheme(dark bool) {
	if p.hwnd == 0 {
		return
	}
	// Win11 uses attribute 20. Windows 10 1809 exposed the same preference as
	// attribute 19; earlier systems ignore both calls and keep their native
	// light/classic caption safely.
	value := uint32(0)
	if dark {
		value = 1
	}
	const (
		dwmwaUseImmersiveDarkMode       = 20
		dwmwaUseImmersiveDarkModeLegacy = 19
	)
	result, _, _ := pDwmSetWindowAttribute.Call(
		uintptr(p.hwnd),
		dwmwaUseImmersiveDarkMode,
		uintptr(unsafe.Pointer(&value)),
		unsafe.Sizeof(value),
	)
	if result != 0 {
		pDwmSetWindowAttribute.Call(
			uintptr(p.hwnd),
			dwmwaUseImmersiveDarkModeLegacy,
			uintptr(unsafe.Pointer(&value)),
			unsafe.Sizeof(value),
		)
	}
}

func (p *panel) triggerOpen(id uint16) bool {
	return p.choice.openID == id
}

func isDangerQuickAction(id uint16) bool { return id == idShutdown || id == idRestart }
