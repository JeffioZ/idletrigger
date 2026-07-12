// Package uicolors supplies the shared semantic palette for native UI surfaces.
package uicolors

// Palette contains Win32 COLORREF values (0x00bbggrr) used by the control panel
// and idle warning. It intentionally models existing UI states rather than a
// general-purpose theming system.
type Palette struct {
	Background, Surface, Hover, Border uint32
	Accent, AccentHover, Pressed       uint32
	Text, MutedText, AccentText        uint32
	Disabled                           uint32
	Danger, DangerHover                uint32
	DangerPressed, DangerText          uint32
	Focus, FocusOnAccent               uint32
}

// ForTheme returns the compact native-control palette for the active Windows
// theme. Accent colors follow the app icon's restrained cyan-blue family while
// preserving readable white text for selected controls.
func ForTheme(dark bool) Palette {
	if dark {
		return Palette{
			Background:    RGB(32, 36, 42),
			Surface:       RGB(43, 48, 54),
			Hover:         RGB(54, 61, 69),
			Border:        RGB(76, 85, 95),
			Accent:        RGB(10, 120, 180),
			AccentHover:   RGB(12, 139, 203),
			Pressed:       RGB(6, 104, 157),
			Text:          RGB(244, 247, 250),
			MutedText:     RGB(174, 182, 191),
			AccentText:    RGB(255, 255, 255),
			Disabled:      RGB(40, 44, 49),
			Danger:        RGB(88, 34, 39),
			DangerHover:   RGB(116, 43, 50),
			DangerPressed: RGB(72, 28, 33),
			DangerText:    RGB(255, 162, 168),
			Focus:         RGB(81, 205, 237),
			FocusOnAccent: RGB(229, 246, 255),
		}
	}
	return Palette{
		Background:    RGB(246, 248, 250),
		Surface:       RGB(255, 255, 255),
		Hover:         RGB(234, 244, 249),
		Border:        RGB(203, 211, 220),
		Accent:        RGB(0, 118, 181),
		AccentHover:   RGB(0, 106, 163),
		Pressed:       RGB(0, 85, 133),
		Text:          RGB(25, 30, 36),
		MutedText:     RGB(99, 108, 118),
		AccentText:    RGB(255, 255, 255),
		Disabled:      RGB(238, 242, 245),
		Danger:        RGB(255, 239, 240),
		DangerHover:   RGB(255, 225, 228),
		DangerPressed: RGB(255, 207, 211),
		DangerText:    RGB(190, 24, 34),
		Focus:         RGB(0, 90, 134),
		FocusOnAccent: RGB(229, 246, 255),
	}
}

// RGB converts RGB channels to the COLORREF format expected by Win32 GDI.
func RGB(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }
