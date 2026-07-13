package popup

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestTimeoutChoices(t *testing.T) {
	choices, selected := timeoutChoices(30, true)
	if len(choices) != 15 || choices[selected].minutes != 30 || choices[selected].label != "30 分钟" {
		t.Fatalf("unexpected preset choices: %#v, selected=%d", choices, selected)
	}

	choices, selected = timeoutChoices(90, false)
	if len(choices) != 16 || choices[selected].minutes != 90 || choices[selected].label != "90 minutes" {
		t.Fatalf("custom timeout was not preserved: %#v, selected=%d", choices, selected)
	}
}

func TestFormatTimeout(t *testing.T) {
	if got := formatTimeout(60, false); got != "1 hour" {
		t.Fatalf("formatTimeout(60) = %q", got)
	}
	if got := formatTimeout(120, true); got != "2 小时" {
		t.Fatalf("formatTimeout(120) = %q", got)
	}
}

func TestVisualStateForButtonRoles(t *testing.T) {
	tests := []struct {
		name                           string
		id                             uint16
		toggleOn, choiceSelected, down bool
		wantRole                       buttonRole
		wantActive                     bool
	}{
		{name: "toggle on", id: idNoSleep, toggleOn: true, wantRole: buttonToggle, wantActive: true},
		{name: "toggle off", id: idTheme, wantRole: buttonToggle, wantActive: false},
		{name: "command remains stateless", id: idIdleAction, toggleOn: true, choiceSelected: true, wantRole: buttonCommand, wantActive: false},
		{name: "system controls remains a command", id: idQuickActions, toggleOn: true, choiceSelected: true, wantRole: buttonCommand, wantActive: false},
		{name: "disabled state is retained", id: idIdleWarning, toggleOn: true, down: true, wantRole: buttonToggle, wantActive: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := visualStateForButton(tt.id, tt.toggleOn, tt.choiceSelected, tt.down)
			if got.Role != tt.wantRole || got.Active != tt.wantActive || got.Disabled != tt.down {
				t.Fatalf("visualStateForButton(%d) = %#v, want role=%d active=%v disabled=%v", tt.id, got, tt.wantRole, tt.wantActive, tt.down)
			}
		})
	}
}

func TestMenuTriggersAreLimitedToHoverMenus(t *testing.T) {
	for _, id := range []uint16{idQuickActions, idLanguage} {
		if !isMenuTrigger(id) {
			t.Fatalf("menu trigger %d was not recognized", id)
		}
	}
	for _, id := range []uint16{idConfig, idExit, idSleep} {
		if isMenuTrigger(id) {
			t.Fatalf("command %d must not use the menu-trigger style", id)
		}
	}
}

func TestButtonRoleMappingCoversEveryPanelAction(t *testing.T) {
	for _, id := range []uint16{idNoSleep, idProcess, idIdle, idIdleWarning, idIdleEnhanced, idTheme, idBattery, idFullscreen, idIPLocation, idHotkeys, idAutostart, idLogging} {
		if got := roleForButton(id); got != buttonToggle {
			t.Fatalf("toggle id %d has role %d", id, got)
		}
	}
	for _, id := range []uint16{idQuickActions, idLock, idSleep, idHibernate, idShutdown, idRestart, idThemeSwitch, idThemeRepair, idConfig, idExit, idTestWarning} {
		if got := roleForButton(id); got != buttonCommand {
			t.Fatalf("command id %d has role %d", id, got)
		}
	}
}

func TestFocusOutlineKeepsSelectedButtonsDistinct(t *testing.T) {
	if focusOutlineUsesLightOnAccent(false) {
		t.Fatal("inactive button should use the standard focus outline")
	}
	if !focusOutlineUsesLightOnAccent(true) {
		t.Fatal("active button should use the dedicated selected-control focus outline")
	}
}

func TestFocusOutlineIsVisibleOnlyDuringKeyboardNavigation(t *testing.T) {
	p := &panel{}
	if p.shouldDrawFocusOutline(odsFocus) {
		t.Fatal("initial mouse-oriented panel should not show a focus outline")
	}
	p.enterKeyboardNavigation()
	if !p.shouldDrawFocusOutline(odsFocus) {
		t.Fatal("keyboard navigation should show the focused control outline")
	}
	p.leaveKeyboardNavigation()
	if p.shouldDrawFocusOutline(odsFocus) {
		t.Fatal("mouse interaction should hide the focus-visible outline without changing focus")
	}
	if p.shouldDrawFocusOutline(0) {
		t.Fatal("an unfocused control must not show a focus outline")
	}
}

func TestComboFocusOutlineRequiresKeyboardFocusedSelectionField(t *testing.T) {
	combo := windows.Handle(120)
	other := windows.Handle(121)
	selectionField := uint32(odsComboBoxEdit)
	tests := []struct {
		name               string
		keyboardNavigation bool
		itemState          uint32
		focused            windows.Handle
		want               bool
	}{
		{name: "tab focus on selection field", keyboardNavigation: true, itemState: selectionField, focused: combo, want: true},
		{name: "initial panel", itemState: selectionField, focused: combo, want: false},
		{name: "mouse focus", itemState: selectionField, focused: combo, want: false},
		{name: "another focused control", keyboardNavigation: true, itemState: selectionField, focused: other, want: false},
		{name: "drop-down item", keyboardNavigation: true, focused: combo, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := comboFocusVisible(tt.keyboardNavigation, tt.itemState, tt.focused, combo); got != tt.want {
				t.Fatalf("comboFocusVisible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWindowIconThemeAndReloadDecisions(t *testing.T) {
	if appIconResourceID != 2 {
		t.Fatalf("class fallback resource = %d, want 2", appIconResourceID)
	}
	if got := windowIconResourceID(false); got != trayDarkIconResourceID {
		t.Fatalf("light theme resource = %d, want %d", got, trayDarkIconResourceID)
	}
	if got := windowIconResourceID(true); got != trayLightIconResourceID {
		t.Fatalf("dark theme resource = %d, want %d", got, trayLightIconResourceID)
	}
	for _, tt := range []struct {
		name                                   string
		initialized, current, requested, force bool
		want                                   bool
	}{
		{name: "initial load", want: true},
		{name: "theme change", initialized: true, current: false, requested: true, want: true},
		{name: "dpi refresh", initialized: true, current: true, requested: true, force: true, want: true},
		{name: "same theme skips reload", initialized: true, current: true, requested: true, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldReloadWindowIcons(tt.initialized, tt.current, tt.requested, tt.force); got != tt.want {
				t.Fatalf("shouldReloadWindowIcons() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLanguageActionKeepsPanelOpenForRefresh(t *testing.T) {
	if actionClosesPanel(ActLanguage) {
		t.Fatal("language switching should keep the panel available for an immediate refresh")
	}
	for _, action := range []Action{ActSleep, ActRestart, ActSwitchTheme, ActRepairTheme, ActConfig, ActExit} {
		if !actionClosesPanel(action) {
			t.Fatalf("action %d should close the panel", action)
		}
	}
}

func TestPanelOriginPinsToWorkAreaBottomRight(t *testing.T) {
	work := rect{Left: 1920, Top: 0, Right: 3840, Bottom: 1040}
	x, y := panelOrigin(work, 720, 600, 16)
	if x != 3104 || y != 424 {
		t.Fatalf("panelOrigin() = (%d, %d), want (3104, 424)", x, y)
	}
}

func TestPanelOriginDoesNotEscapeSmallWorkArea(t *testing.T) {
	work := rect{Left: 100, Top: 50, Right: 500, Bottom: 300}
	x, y := panelOrigin(work, 720, 600, 16)
	if x != work.Left || y != work.Top {
		t.Fatalf("panelOrigin() = (%d, %d), want (%d, %d)", x, y, work.Left, work.Top)
	}
}

func TestOwnedBrushesIncludesEveryPanelBrush(t *testing.T) {
	p := &panel{}
	p.dangerPressedBorderBrush = windows.Handle(21)
	brushes := p.ownedBrushes()
	if len(brushes) != 21 {
		t.Fatalf("owned brush count = %d, want 21", len(brushes))
	}
	for _, brush := range brushes {
		if brush == p.dangerPressedBorderBrush {
			return
		}
	}
	t.Fatal("danger pressed border brush is missing from the release inventory")
}

func TestPopupMetricsUseOneDPITransform(t *testing.T) {
	metrics := newPopupMetrics(defaultPopupStyle, 1.5)
	if got := metrics.px(metrics.style.Layout.PanelWidth); got != 708 {
		t.Fatalf("scaled panel width = %d, want 708", got)
	}
	if got := metrics.px(metrics.style.Control.ToggleBoxSize); got != 24 {
		t.Fatalf("scaled toggle box = %d, want 24", got)
	}
}

func TestExplicitThemeDoesNotReadSystemTheme(t *testing.T) {
	if (&panel{theme: ThemeLight}).resolveTheme() {
		t.Fatal("explicit light theme resolved as dark")
	}
	if !(&panel{theme: ThemeDark}).resolveTheme() {
		t.Fatal("explicit dark theme resolved as light")
	}
}

func TestControlStateCombinesModelAndNativeState(t *testing.T) {
	p := &panel{
		toggles:            map[uint16]bool{idIdle: true},
		selected:           map[uint16]bool{},
		disabled:           map[uint16]bool{},
		hoverID:            idIdle,
		keyboardNavigation: true,
	}
	state := p.controlState(idIdle, odsSelected|odsFocus)
	if !state.Active || !state.Hovered || !state.Pressed || !state.Focused || state.Disabled {
		t.Fatalf("control state = %#v", state)
	}
}
