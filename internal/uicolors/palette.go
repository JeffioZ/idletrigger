// Package uicolors supplies the shared semantic palette for native UI surfaces.
package uicolors

// Palette contains Win32 COLORREF values (0x00bbggrr) used by the control panel
// and idle warning. It intentionally models existing UI states rather than a
// general-purpose theming system.
type Palette struct {
	WindowBackground, Surface, ElevatedSurface, HoverSurface uint32
	Border, SubtleBorder                                     uint32
	PrimaryText, SecondaryText                               uint32
	MutedText, DisabledText                                  uint32
	DisabledSurface                                          uint32
	Accent, AccentHover, AccentPressed                       uint32
	Selected, SelectedHover                                  uint32
	AccentText                                               uint32
	Focus, FocusOnAccent                                     uint32
	DangerBackground, DangerHover, DangerPressed             uint32
	DangerBorder, DangerHoverBorder, DangerPressedBorder     uint32
	DangerText, DangerFocus                                  uint32
	CloseText, CloseHover, ClosePressed                      uint32
	CloseActiveText                                          uint32
	TooltipBackground, TooltipText                           uint32
}

// ForTheme returns the compact native-control palette for the active Windows
// theme. Accent colors follow the app icon's restrained cyan-blue family while
// preserving readable white text for selected controls.
func ForTheme(dark bool) Palette {
	if dark {
		return Palette{
			WindowBackground:    RGB(32, 36, 42),
			Surface:             RGB(43, 48, 54),
			ElevatedSurface:     RGB(52, 59, 67),
			HoverSurface:        RGB(54, 61, 69),
			Border:              RGB(76, 85, 95),
			SubtleBorder:        RGB(60, 68, 77),
			PrimaryText:         RGB(244, 247, 250),
			SecondaryText:       RGB(204, 212, 220),
			MutedText:           RGB(170, 181, 191),
			DisabledText:        RGB(119, 130, 141),
			DisabledSurface:     RGB(40, 44, 49),
			Accent:              RGB(10, 120, 180),
			AccentHover:         RGB(12, 139, 203),
			AccentPressed:       RGB(6, 104, 157),
			Selected:            RGB(10, 120, 180),
			SelectedHover:       RGB(12, 139, 203),
			AccentText:          RGB(255, 255, 255),
			Focus:               RGB(81, 205, 237),
			FocusOnAccent:       RGB(229, 246, 255),
			DangerBackground:    RGB(59, 43, 50),
			DangerHover:         RGB(93, 52, 67),
			DangerPressed:       RGB(68, 38, 48),
			DangerBorder:        RGB(123, 72, 88),
			DangerHoverBorder:   RGB(166, 91, 112),
			DangerPressedBorder: RGB(136, 70, 87),
			DangerText:          RGB(255, 201, 211),
			DangerFocus:         RGB(255, 181, 194),
			CloseText:           RGB(182, 199, 212),
			CloseHover:          RGB(80, 57, 68),
			ClosePressed:        RGB(62, 45, 53),
			CloseActiveText:     RGB(242, 206, 215),
			TooltipBackground:   RGB(52, 59, 67),
			TooltipText:         RGB(244, 247, 250),
		}
	}
	return Palette{
		WindowBackground:    RGB(246, 248, 250),
		Surface:             RGB(255, 255, 255),
		ElevatedSurface:     RGB(251, 253, 255),
		HoverSurface:        RGB(234, 244, 249),
		Border:              RGB(203, 211, 220),
		SubtleBorder:        RGB(225, 231, 237),
		PrimaryText:         RGB(25, 30, 36),
		SecondaryText:       RGB(70, 82, 94),
		MutedText:           RGB(99, 108, 118),
		DisabledText:        RGB(126, 137, 147),
		DisabledSurface:     RGB(238, 242, 245),
		Accent:              RGB(0, 118, 181),
		AccentHover:         RGB(0, 106, 163),
		AccentPressed:       RGB(0, 85, 133),
		Selected:            RGB(0, 118, 181),
		SelectedHover:       RGB(0, 106, 163),
		AccentText:          RGB(255, 255, 255),
		Focus:               RGB(0, 90, 134),
		FocusOnAccent:       RGB(229, 246, 255),
		DangerBackground:    RGB(255, 247, 248),
		DangerHover:         RGB(255, 220, 226),
		DangerPressed:       RGB(244, 196, 208),
		DangerBorder:        RGB(229, 183, 193),
		DangerHoverBorder:   RGB(217, 139, 156),
		DangerPressedBorder: RGB(188, 101, 119),
		DangerText:          RGB(146, 44, 62),
		DangerFocus:         RGB(162, 59, 77),
		CloseText:           RGB(84, 104, 121),
		CloseHover:          RGB(244, 231, 235),
		ClosePressed:        RGB(235, 205, 213),
		CloseActiveText:     RGB(123, 58, 75),
		TooltipBackground:   RGB(251, 253, 255),
		TooltipText:         RGB(25, 30, 36),
	}
}

// RGB converts RGB channels to the COLORREF format expected by Win32 GDI.
func RGB(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }
