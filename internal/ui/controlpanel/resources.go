package controlpanel

import (
	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/ui/colors"
)

// panelResources is the complete set of native resources owned by one panel.
// HWND children, including the tooltip, are owned by the panel HWND and are
// destroyed by Windows with their parent. Fonts, brushes, and icons below are
// created by IdleTrigger and must be explicitly released exactly once.
type panelResources struct {
	font, sectionFont, subtitleFont, choiceSelectedFont windows.Handle

	backgroundBrush, elevatedBrush windows.Handle
	surfaceBrush                   windows.Handle
	hoverBrush                     windows.Handle
	borderBrush                    windows.Handle
	subtleBorderBrush              windows.Handle
	accentBrush                    windows.Handle
	accentHoverBrush               windows.Handle
	pressedBrush                   windows.Handle
	selectedBrush                  windows.Handle
	selectedHoverBrush             windows.Handle
	disabledBrush                  windows.Handle
	focusBrush                     windows.Handle
	focusOnAccentBrush             windows.Handle
	dangerBrush                    windows.Handle
	dangerHoverBrush               windows.Handle
	dangerPressedBrush             windows.Handle
	dangerBorderBrush              windows.Handle
	dangerHoverBorderBrush         windows.Handle
	dangerPressedBorderBrush       windows.Handle
	dangerFocusBrush               windows.Handle

	largeIcon, smallIcon windows.Handle
}

func (p *panel) ownedBrushes() []windows.Handle {
	return []windows.Handle{
		p.backgroundBrush, p.elevatedBrush, p.surfaceBrush, p.hoverBrush,
		p.borderBrush, p.subtleBorderBrush, p.accentBrush, p.accentHoverBrush,
		p.pressedBrush, p.selectedBrush, p.selectedHoverBrush, p.disabledBrush,
		p.focusBrush, p.focusOnAccentBrush, p.dangerBrush, p.dangerHoverBrush,
		p.dangerPressedBrush, p.dangerBorderBrush, p.dangerHoverBorderBrush,
		p.dangerPressedBorderBrush, p.dangerFocusBrush,
	}
}

func (p *panel) releaseBrushes() {
	for _, brush := range p.ownedBrushes() {
		if brush != 0 {
			pDeleteObject.Call(uintptr(brush))
		}
	}
	p.backgroundBrush = 0
	p.elevatedBrush = 0
	p.surfaceBrush = 0
	p.hoverBrush = 0
	p.borderBrush = 0
	p.subtleBorderBrush = 0
	p.accentBrush = 0
	p.accentHoverBrush = 0
	p.pressedBrush = 0
	p.selectedBrush = 0
	p.selectedHoverBrush = 0
	p.disabledBrush = 0
	p.focusBrush = 0
	p.focusOnAccentBrush = 0
	p.dangerBrush = 0
	p.dangerHoverBrush = 0
	p.dangerPressedBrush = 0
	p.dangerBorderBrush = 0
	p.dangerHoverBorderBrush = 0
	p.dangerPressedBorderBrush = 0
	p.dangerFocusBrush = 0
}

func (p *panel) rebuildBrushes(palette colors.Palette) {
	p.releaseBrushes()
	p.backgroundBrush = makeBrush(palette.WindowBackground)
	p.elevatedBrush = makeBrush(palette.ElevatedSurface)
	p.surfaceBrush = makeBrush(palette.Surface)
	p.hoverBrush = makeBrush(palette.HoverSurface)
	p.borderBrush = makeBrush(palette.Border)
	p.subtleBorderBrush = makeBrush(palette.SubtleBorder)
	p.accentBrush = makeBrush(palette.Accent)
	p.accentHoverBrush = makeBrush(palette.AccentHover)
	p.pressedBrush = makeBrush(palette.AccentPressed)
	p.selectedBrush = makeBrush(palette.Selected)
	p.selectedHoverBrush = makeBrush(palette.SelectedHover)
	p.disabledBrush = makeBrush(palette.DisabledSurface)
	p.focusBrush = makeBrush(palette.Focus)
	p.focusOnAccentBrush = makeBrush(palette.FocusOnAccent)
	p.dangerBrush = makeBrush(palette.DangerBackground)
	p.dangerHoverBrush = makeBrush(palette.DangerHover)
	p.dangerPressedBrush = makeBrush(palette.DangerPressed)
	p.dangerBorderBrush = makeBrush(palette.DangerBorder)
	p.dangerHoverBorderBrush = makeBrush(palette.DangerHoverBorder)
	p.dangerPressedBorderBrush = makeBrush(palette.DangerPressedBorder)
	p.dangerFocusBrush = makeBrush(palette.DangerFocus)
}

func (p *panel) releaseFonts() {
	for _, font := range []windows.Handle{p.font, p.sectionFont, p.subtitleFont, p.choiceSelectedFont} {
		if font != 0 {
			pDeleteObject.Call(uintptr(font))
		}
	}
	p.font, p.sectionFont, p.subtitleFont, p.choiceSelectedFont = 0, 0, 0, 0
}

func (p *panel) releaseIcons() {
	for _, icon := range []windows.Handle{p.largeIcon, p.smallIcon} {
		if icon != 0 {
			pDestroyIcon.Call(uintptr(icon))
		}
	}
	p.largeIcon, p.smallIcon = 0, 0
	p.iconsInitialized = false
	p.iconThemeDark = false
}

// releaseNativeResources is idempotent and is called only after the panel is
// no longer active. It never destroys the HWND itself; its caller owns that
// lifecycle boundary.
func (p *panel) releaseNativeResources() {
	p.releaseFonts()
	p.releaseBrushes()
	p.releaseIcons()
}

func makeBrush(color uint32) windows.Handle {
	result, _, _ := pCreateBrush.Call(uintptr(color))
	return windows.Handle(result)
}
