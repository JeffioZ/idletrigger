package controlpanel

import (
	"github.com/JeffioZ/idletrigger/internal/platform/windows/gdiplus"
	"github.com/JeffioZ/idletrigger/internal/ui/trayicon"
	"golang.org/x/sys/windows"
	"unsafe"
)

func (p *panel) drawToggle(item *drawItem) {
	state := p.controlState(uint16(item.CtlID), item.ItemState)
	pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&item.Rect)), uintptr(p.backgroundBrush))
	boxSize := int32(p.sc(p.metrics.style.Control.ToggleBoxSize))
	box := rect{
		Left:   item.Rect.Left + int32(p.sc(p.metrics.style.Control.ToggleLeftInset)),
		Top:    item.Rect.Top + (item.Rect.Bottom-item.Rect.Top-boxSize)/2,
		Right:  item.Rect.Left + int32(p.sc(p.metrics.style.Control.ToggleLeftInset)) + boxSize,
		Bottom: item.Rect.Top + (item.Rect.Bottom-item.Rect.Top-boxSize)/2 + boxSize,
	}
	brush, border, textColor := p.surfaceBrush, p.borderBrush, p.palette.PrimaryText
	// Keep label text stable for every interactive state. The checkbox box
	// alone communicates hover, press, and checked state in the quiet native
	// control language; disabled is the only text de-emphasis case.
	if state.Disabled || item.ItemState&odsDisabled != 0 {
		brush, border, textColor = p.disabledBrush, p.subtleBorderBrush, p.palette.DisabledText
	} else if state.Pressed {
		if state.Active {
			brush, border = p.pressedBrush, p.pressedBrush
		} else {
			brush, border = p.elevatedBrush, p.pressedBrush
		}
	} else if state.Active && state.Hovered {
		brush, border = p.accentHoverBrush, p.accentHoverBrush
	} else if state.Active {
		brush, border = p.accentBrush, p.accentBrush
	} else if state.Hovered {
		brush, border = p.hoverBrush, p.accentHoverBrush
	}
	p.drawToggleBox(item.HDC, box, brush, border)
	old, _, _ := pSelectObject.Call(uintptr(item.HDC), uintptr(p.font))
	defer pSelectObject.Call(uintptr(item.HDC), old)
	pSetBkMode.Call(uintptr(item.HDC), transparent)
	if state.Active {
		check, _ := windows.UTF16PtrFromString("✓")
		checkColor := p.palette.AccentText
		if state.Disabled || item.ItemState&odsDisabled != 0 {
			// White on the light disabled surface has insufficient contrast.
			checkColor = p.palette.MutedText
		}
		checkResult := gdiplus.DrawCheck(item.HDC, box.Left, box.Top, box.Right, box.Bottom, checkColor, int32(p.sc(2)))
		if checkResult != gdiplus.DrawCompleted {
			if checkResult == gdiplus.DrawMayBeDirty {
				p.drawToggleBox(item.HDC, box, brush, border)
			}
			pSetTextColor.Call(uintptr(item.HDC), uintptr(checkColor))
			pDrawText.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(check)), ^uintptr(0), uintptr(unsafe.Pointer(&box)), dtCenter|dtVCenter|dtSingleLine)
		}
	}
	if state.Focused {
		focus := item.Rect
		inset := int32(p.sc(p.metrics.style.Control.FocusInset))
		focus.Left += inset
		focus.Top += inset
		focus.Right -= inset
		focus.Bottom -= inset
		if focus.Left < focus.Right && focus.Top < focus.Bottom {
			pFrameRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&focus)), uintptr(p.focusBrush))
		}
	}
	text, _ := windows.UTF16PtrFromString(p.labels[uint16(item.CtlID)])
	bounds := item.Rect
	bounds.Left = box.Right + int32(p.sc(p.metrics.style.Control.ToggleTextGap))
	bounds.Right -= int32(p.sc(p.metrics.style.Control.MenuSurfaceInset))
	pSetTextColor.Call(uintptr(item.HDC), uintptr(textColor))
	drawTextLeftCentered(item.HDC, text, bounds)
}

func (p *panel) drawToggleBox(dc windows.Handle, box rect, brush, border windows.Handle) {
	pFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(&box)), uintptr(brush))
	pFrameRect.Call(uintptr(dc), uintptr(unsafe.Pointer(&box)), uintptr(border))
}

func (p *panel) drawStatic(item *drawItem) {
	id := uint16(item.CtlID)
	kind := p.staticKinds[id]
	if kind == staticQuickMenu {
		p.roundRect(item.HDC, item.Rect, p.elevatedBrush, p.palette.SubtleBorder, p.sc(p.metrics.style.Control.CornerRadius))
		return
	}
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
