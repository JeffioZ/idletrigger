package controlpanel

import "github.com/JeffioZ/idletrigger/internal/feature/theme"

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
	QuickMenuRowHeight, QuickMenuRowGap            int
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

// panelStyle is a fixed visual vocabulary for IdleTrigger's control panel,
// not a user-configurable skin or reusable UI framework.
type panelStyle struct {
	Layout  layoutTokens
	Fonts   fontTokens
	Control controlTokens
}

var defaultPanelStyle = panelStyle{
	Layout: layoutTokens{
		PanelWidth: 472, Padding: 18, Gap: 8, SectionGap: 14, LabelGap: 2,
		ButtonHeight: 36, SectionHeight: 22, SubtitleHeight: 18,
		QuickMenuRowHeight: 34, QuickMenuRowGap: 1,
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

// panelMetrics keeps layout and drawing on one 96-DPI logical-pixel rule.
type panelMetrics struct {
	style panelStyle
	scale float64
}

func newPanelMetrics(style panelStyle, scale float64) panelMetrics {
	if scale <= 0 {
		scale = 1
	}
	return panelMetrics{style: style, scale: scale}
}

func (m panelMetrics) px(value int) int { return int(float64(value)*m.scale + 0.5) }

func (p *panel) resolveTheme() bool {
	switch p.theme {
	case ThemeLight:
		return false
	case ThemeDark:
		return true
	default:
		return theme.Current() == theme.ModeDark
	}
}
