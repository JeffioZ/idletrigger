package controlpanel

import (
	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
	"github.com/JeffioZ/idletrigger/internal/ui/trayicon"
	"golang.org/x/sys/windows"
	"unsafe"
)

func (p *panel) drawToggle(item *drawItem) {
	state := p.controlState(uint16(item.CtlID), item.ItemState)
	nativeform.DrawCheckbox(item.HDC, nativeRect(item.Rect), p.font, p.labels[uint16(item.CtlID)], p.palette, p.palette.WindowBackground, nativeControlState(state), p.metrics.scale)
}

func (p *panel) drawStatic(item *drawItem) {
	id := uint16(item.CtlID)
	kind := p.staticKinds[id]
	pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&item.Rect)), uintptr(p.backgroundBrush))
	pSetBkMode.Call(uintptr(item.HDC), transparent)
	text, err := windows.UTF16PtrFromString(p.labels[id])
	if err != nil {
		return
	}
	bounds := item.Rect
	if kind == staticSection {
		accent := bounds
		accent.Right = accent.Left + int32(p.sc(3))
		// Match the rendered title glyph height while keeping the accent centered
		// in its row at every DPI scale.
		accentHeight := int32(p.sc(16))
		accent.Top += (accent.Bottom - accent.Top - accentHeight) / 2
		accent.Bottom = accent.Top + accentHeight
		pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&accent)), uintptr(p.accentBrush))
		bounds.Left += int32(p.sc(10))
		pSetTextColor.Call(uintptr(item.HDC), uintptr(p.palette.PrimaryText))
		old, _, _ := pSelectObject.Call(uintptr(item.HDC), uintptr(p.sectionFont))
		defer pSelectObject.Call(uintptr(item.HDC), old)

		// Measure the translated title so the divider follows its actual width.
		// This keeps Chinese, English, and future locales equally balanced.
		measured := bounds
		pDrawText.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&measured)), dtLeft|dtVCenter|dtCalcRect)
		separator := bounds
		separator.Left = measured.Right + int32(p.sc(14))
		separator.Top += int32(p.sc(10))
		separator.Bottom = separator.Top + 1
		if separator.Left < separator.Right {
			pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&separator)), uintptr(p.subtleBorderBrush))
		}
	} else {
		pSetTextColor.Call(uintptr(item.HDC), uintptr(p.palette.MutedText))
		old, _, _ := pSelectObject.Call(uintptr(item.HDC), uintptr(p.subtitleFont))
		defer pSelectObject.Call(uintptr(item.HDC), old)
	}
	pDrawText.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), dtLeft|dtVCenter|dtWordBreak)
}

func (p *panel) drawItemBuffered(item *drawItem) {
	if item == nil || item.HDC == 0 {
		return
	}
	bounds := nativeform.Rect{Left: item.Rect.Left, Top: item.Rect.Top, Right: item.Rect.Right, Bottom: item.Rect.Bottom}
	paint := func(dc windows.Handle, local nativeform.Rect) {
		buffered := *item
		buffered.HDC = dc
		buffered.Rect = rect{Left: local.Left, Top: local.Top, Right: local.Right, Bottom: local.Bottom}
		if p.staticKinds[uint16(buffered.CtlID)] != staticNone {
			p.drawStatic(&buffered)
		} else {
			p.drawButton(&buffered)
		}
	}
	if nativeform.DrawBuffered(item.HDC, bounds, paint) {
		return
	}
	if p.staticKinds[uint16(item.CtlID)] != staticNone {
		p.drawStatic(item)
	} else {
		p.drawButton(item)
	}
}

func drawTextCentered(dc windows.Handle, text *uint16, bounds rect) {
	measure := rect{Left: bounds.Left, Top: bounds.Top, Right: bounds.Right, Bottom: bounds.Bottom}
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&measure)), dtCenter|dtWordBreak|dtCalcRect)
	textH := measure.Bottom - measure.Top
	if textH < bounds.Bottom-bounds.Top {
		bounds.Top += ((bounds.Bottom - bounds.Top) - textH) / 2
	}
	// bounds.Top already centers the measured text block. Applying DT_VCENTER
	// again would shift single-line labels downward within the reduced bounds.
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), dtCenter|dtWordBreak)
}

func drawTextLeftCentered(dc windows.Handle, text *uint16, bounds rect) {
	measure := bounds
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&measure)), dtLeft|dtWordBreak|dtCalcRect)
	textH := measure.Bottom - measure.Top
	if textH < bounds.Bottom-bounds.Top {
		bounds.Top += ((bounds.Bottom - bounds.Top) - textH) / 2
	}
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), dtLeft|dtWordBreak)
}

func (p *panel) fill(dc, brush windows.Handle) {
	var r rect
	pGetClientRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&r)))
	pFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(&r)), uintptr(brush))
}

func panelFor(hwnd windows.Handle) *panel {
	panelMu.Lock()
	defer panelMu.Unlock()
	if active != nil && active.hwnd == hwnd {
		return active
	}
	return nil
}

func panelForButton(hwnd windows.Handle) *panel {
	panelMu.Lock()
	defer panelMu.Unlock()
	if active == nil {
		return nil
	}
	for _, control := range active.controls {
		if control == hwnd {
			return active
		}
	}
	return nil
}

func clearPanel(p *panel, hwnd windows.Handle) {
	panelMu.Lock()
	if active != p || (hwnd != 0 && p.hwnd != hwnd) {
		panelMu.Unlock()
		return
	}
	active = nil
	panelMu.Unlock()
	p.closeChoice(false)
	trayicon.ClearTabNavigationWindow(p.hwnd)
	for hwnd, old := range p.oldButtonProc {
		if hwnd != 0 && old != 0 {
			setWindowProc(hwnd, old)
		}
	}
	p.releaseNativeResources()
	p.hwnd = 0
}
