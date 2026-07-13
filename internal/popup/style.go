package popup

import "github.com/JeffioZ/idletrigger/internal/themeswitch"

// Theme selects the visual source for one panel. ThemeFollowSystem preserves
// normal application behavior; explicit values are reserved for deterministic
// hosts such as the future screenshot command.
type Theme uint8

const (
	ThemeFollowSystem Theme = iota
	ThemeLight
	ThemeDark
)

type layoutTokens struct {
	PanelWidth, Padding, Gap, SectionGap, LabelGap int
	ButtonHeight, SectionHeight, SubtitleHeight    int
	QuickMenuRowHeight                             int
}

type fontTokens struct {
	BodySize, SectionSize, SubtitleSize       int32
	BodyWeight, SectionWeight, SubtitleWeight int32
}

type controlTokens struct {
	CornerRadius, ToggleBoxSize                     int
	ButtonTextInset, ToggleTextGap, ToggleLeftInset int
	FocusInset, FocusRingWidth, MenuFocusInset      int
	ArrowWidth, ArrowHeight, SelectedMarkerWidth    int
	MenuHintWidth, MenuHintHeight, MenuSurfaceInset int
	MenuSurfaceWidthCompensation                    int
	IconLarge, IconSmall                            int
}

// popupStyle is a fixed visual vocabulary for IdleTrigger's control panel,
// not a user-configurable skin or reusable UI framework.
type popupStyle struct {
	Layout  layoutTokens
	Fonts   fontTokens
	Control controlTokens
}

var defaultPopupStyle = popupStyle{
	Layout: layoutTokens{
		PanelWidth: 472, Padding: 18, Gap: 8, SectionGap: 14, LabelGap: 2,
		ButtonHeight: 36, SectionHeight: 22, SubtitleHeight: 18, QuickMenuRowHeight: 34,
	},
	Fonts: fontTokens{
		BodySize: 14, SectionSize: 14, SubtitleSize: 12,
		BodyWeight: 400, SectionWeight: 700, SubtitleWeight: 600,
	},
	Control: controlTokens{
		CornerRadius: 6, ToggleBoxSize: 16, ButtonTextInset: 8, ToggleTextGap: 8, ToggleLeftInset: 2,
		FocusInset: 2, FocusRingWidth: 2, MenuFocusInset: 3,
		ArrowWidth: 8, ArrowHeight: 4, SelectedMarkerWidth: 3,
		MenuHintWidth: 28, MenuHintHeight: 1, MenuSurfaceInset: 4,
		MenuSurfaceWidthCompensation: 1,
		IconLarge:                    32, IconSmall: 16,
	},
}

// popupMetrics keeps layout and drawing on one 96-DPI logical-pixel rule.
type popupMetrics struct {
	style popupStyle
	scale float64
}

func newPopupMetrics(style popupStyle, scale float64) popupMetrics {
	if scale <= 0 {
		scale = 1
	}
	return popupMetrics{style: style, scale: scale}
}

func (m popupMetrics) px(value int) int { return int(float64(value)*m.scale + 0.5) }

func (p *panel) resolveTheme() bool {
	switch p.theme {
	case ThemeLight:
		return false
	case ThemeDark:
		return true
	default:
		return themeswitch.Current() == themeswitch.ModeDark
	}
}
