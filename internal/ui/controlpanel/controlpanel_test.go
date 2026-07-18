package controlpanel

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/JeffioZ/idletrigger/internal/ui/colors"
	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
	"golang.org/x/sys/windows"
)

func TestAutomationTooltipFormatsCountSummaryAndExplanation(t *testing.T) {
	texts := map[string]string{
		"tip_automation_status": "%d enabled\n%s\n%s",
		"tip_automation":        "Built-in tasks only.",
	}
	p := panel{automationCount: 2, automationSummary: "Next: 23:00", lang: func(key string) string { return texts[key] }}
	got := p.tooltipText(idAutomation)
	if got != "2 enabled\nNext: 23:00\nBuilt-in tasks only." || strings.Contains(got, "%!") {
		t.Fatalf("automation tooltip = %q", got)
	}
}

func TestPowerManagementTooltipSeparatesManualAndRuntimeState(t *testing.T) {
	texts := map[string]string{
		"tip_power_setting_status": "Manual setting: %s.\nRuntime status: %s.\n%s",
		"tip_state_enabled":        "on",
		"tip_state_disabled":       "off",
		"tip_nosleep":              "Stay Awake help.",
		"tip_idle":                 "Idle help.",
		"status_unknown":           "Unknown",
	}
	p := panel{
		lang:          func(key string) string { return texts[key] },
		noSleepStatus: "Enabled by an automatic task",
		idleStatus:    "Paused by Stay Awake",
		toggles:       map[uint16]bool{idNoSleep: false, idIdle: true},
		disabled:      map[uint16]bool{},
	}
	if got := p.tooltipText(idNoSleep); got != "Manual setting: off.\nRuntime status: Enabled by an automatic task.\nStay Awake help." {
		t.Fatalf("Stay Awake tooltip = %q", got)
	}
	if got := p.tooltipText(idIdle); got != "Manual setting: on.\nRuntime status: Paused by Stay Awake.\nIdle help." {
		t.Fatalf("idle tooltip = %q", got)
	}
}

func TestProjectHomeLinkColorUsesASeparateLightHoverRamp(t *testing.T) {
	light := colors.ForTheme(false)
	dark := colors.ForTheme(true)

	if got := projectHomeLinkColor(light, false, buttonVisualState{}, 0); got != light.Accent {
		t.Fatalf("light normal color = %#x, want %#x", got, light.Accent)
	}
	if got := projectHomeLinkColor(light, false, buttonVisualState{Hovered: true}, 0); got != colors.RGB(0, 90, 158) {
		t.Fatalf("light hover color = %#x", got)
	}
	if got := projectHomeLinkColor(light, false, buttonVisualState{Pressed: true}, 0); got != colors.RGB(0, 60, 102) {
		t.Fatalf("light pressed color = %#x", got)
	}
	if got := projectHomeLinkColor(dark, true, buttonVisualState{Hovered: true}, 0); got != dark.AccentHover {
		t.Fatalf("dark hover color = %#x, want %#x", got, dark.AccentHover)
	}
	if got := projectHomeLinkColor(dark, true, buttonVisualState{Pressed: true, Disabled: true}, 0); got != dark.DisabledText {
		t.Fatalf("disabled color = %#x, want %#x", got, dark.DisabledText)
	}
}

func TestTimeoutChoices(t *testing.T) {
	choices, selected := timeoutChoices(30, true)
	if len(choices) != 10 || choices[selected].minutes != 30 || choices[selected].label != "30 分钟" {
		t.Fatalf("unexpected preset choices: %#v, selected=%d", choices, selected)
	}

	choices, selected = timeoutChoices(90, false)
	if len(choices) != 10 || choices[selected].minutes != 30 || choices[selected].label != "30 minutes" {
		t.Fatalf("unsupported timeout was not normalized: %#v, selected=%d", choices, selected)
	}
}

func TestChoiceSelectionModelAppliesByOwnerAndIndex(t *testing.T) {
	var action Action
	var value int
	p := &panel{
		onAction: func(next Action, nextValue int) { action, value = next, nextValue },
		labels:   map[uint16]string{},
		choice: choiceSurface{
			options:  map[uint16][]string{idIdleAction: {"Sleep", "Shutdown"}},
			selected: map[uint16]int{idIdleAction: 0},
		},
	}
	p.applyChoice(idIdleAction, 1)
	if p.choice.selected[idIdleAction] != 1 || p.labels[idIdleAction] != "Shutdown" {
		t.Fatalf("selection was not applied: selected=%d label=%q", p.choice.selected[idIdleAction], p.labels[idIdleAction])
	}
	if action != ActIdleAction || value != 1 {
		t.Fatalf("selection callback = (%v, %d)", action, value)
	}
	p.applyChoice(idIdleAction, 1)
	if value != 1 {
		t.Fatal("reapplying the selected row should remain a no-op")
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

func TestControlStateRetainsInteractiveFlags(t *testing.T) {
	p := &panel{
		toggles:            map[uint16]bool{idNoSleep: true},
		selected:           map[uint16]bool{},
		disabled:           map[uint16]bool{idNoSleep: true},
		hoverID:            idNoSleep,
		keyboardNavigation: true,
	}
	state := p.controlState(idNoSleep, odsSelected|odsFocus)
	if !state.Active || !state.Hovered || !state.Pressed || !state.Disabled || !state.Focused {
		t.Fatalf("control state lost an existing interaction flag: %#v", state)
	}
}

func TestControlStateIncludesNativeDisabledFlag(t *testing.T) {
	p := &panel{toggles: map[uint16]bool{}, selected: map[uint16]bool{}, disabled: map[uint16]bool{}}
	if state := p.controlState(idConfig, odsDisabled); !state.Disabled {
		t.Fatalf("native disabled flag was lost: %#v", state)
	}
}

func TestMenuTriggersAreLimitedToClickMenus(t *testing.T) {
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

func TestTriggerOpenUsesOnlyItsRealMenuState(t *testing.T) {
	p := &panel{}
	if p.triggerOpen(idQuickActions) || p.triggerOpen(idLanguage) || p.triggerOpen(idIdleTimeout) {
		t.Fatal("fresh panel must not report any trigger as open")
	}
	p.choice.openID = idQuickActions
	if !p.triggerOpen(idQuickActions) || p.triggerOpen(idLanguage) {
		t.Fatal("quick menu open state was not isolated")
	}
	p.choice.openID = idLanguage
	if p.triggerOpen(idQuickActions) || !p.triggerOpen(idLanguage) {
		t.Fatal("language menu open state was not isolated")
	}
	p.choice.openID = idIdleAction
	if p.triggerOpen(idIdleTimeout) || !p.triggerOpen(idIdleAction) {
		t.Fatal("choice trigger must use its matching open ID")
	}
}

func TestDangerQuickActionsAreLimitedToShutdownAndRestart(t *testing.T) {
	for _, id := range []uint16{idLock, idSleep, idHibernate} {
		if isDangerQuickAction(id) {
			t.Fatalf("ordinary quick action %d was marked dangerous", id)
		}
	}
	for _, id := range []uint16{idShutdown, idRestart} {
		if !isDangerQuickAction(id) {
			t.Fatalf("danger quick action %d was not marked dangerous", id)
		}
	}
}

func TestPopupMenuItemsKeepOnlySemanticDifferences(t *testing.T) {
	p := &panel{lang: func(key string) string { return key }, selected: map[uint16]bool{idLangZH: true}}
	quick := p.quickMenuItems()
	if len(quick) != len(quickActionIDs()) {
		t.Fatalf("quick items = %d", len(quick))
	}
	for _, item := range quick {
		wantDanger := item.Value == idShutdown || item.Value == idRestart
		if item.Danger != wantDanger {
			t.Fatalf("quick item %+v danger = %v, want %v", item, item.Danger, wantDanger)
		}
	}
	languages, selected := p.languageMenuItems()
	if len(languages) != 2 || selected != 1 || languages[0].Danger || languages[1].Danger {
		t.Fatalf("language items = %+v, selected = %d", languages, selected)
	}
}

func TestButtonRoleMappingCoversEveryPanelAction(t *testing.T) {
	for _, id := range []uint16{idNoSleep, idAutomationEnabled, idIdle, idIdleWarning, idIdleEnhanced, idTheme, idBattery, idFullscreen, idIPLocation, idHotkeys, idAutostart, idLogging} {
		if got := roleForButton(id); got != buttonToggle {
			t.Fatalf("toggle id %d has role %d", id, got)
		}
	}
	for _, id := range []uint16{idQuickActions, idAutomation, idLock, idSleep, idHibernate, idShutdown, idRestart, idThemeSwitch, idThemeRepair, idConfig, idProjectHome, idExit, idTestWarning} {
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

func TestRefreshActionsKeepPanelOpen(t *testing.T) {
	for _, action := range []Action{ActLanguage, ActSwitchTheme, ActRepairTheme} {
		if actionClosesPanel(action) {
			t.Fatalf("action %d should keep the panel available for an immediate refresh", action)
		}
	}
	for _, action := range []Action{ActSleep, ActRestart, ActConfig, ActExit} {
		if !actionClosesPanel(action) {
			t.Fatalf("action %d should close the panel", action)
		}
	}
	if actionClosesPanel(ActProjectHome) {
		t.Fatal("project home should keep the panel open")
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

func TestPanelFallbackCoordinateCannotReachTheDesktop(t *testing.T) {
	if panelFallbackWindowCoordinate > -30000 {
		t.Fatalf("panel fallback coordinate %d is not safely outside the desktop", panelFallbackWindowCoordinate)
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
	metrics := newPanelMetrics(defaultPanelStyle, 1.5)
	if got := metrics.px(metrics.style.Layout.PanelWidth); got != 708 {
		t.Fatalf("scaled panel width = %d, want 708", got)
	}
	if got := metrics.px(metrics.style.Control.ToggleBoxSize); got != 24 {
		t.Fatalf("scaled toggle box = %d, want 24", got)
	}
}

func TestSuggestedDPIBoundsUsesTheSystemRectangle(t *testing.T) {
	want := rect{Left: -640, Top: 80, Right: 120, Bottom: 680}
	got, ok := suggestedDPIBounds(uintptr(unsafe.Pointer(&want)))
	if !ok || got != want {
		t.Fatalf("suggested DPI bounds = (%+v, %v), want (%+v, true)", got, ok, want)
	}
	invalid := rect{Left: 10, Top: 10, Right: 10, Bottom: 20}
	if _, ok := suggestedDPIBounds(uintptr(unsafe.Pointer(&invalid))); ok {
		t.Fatal("empty suggested DPI bounds must use the fallback placement")
	}
}

func TestPanelScaleUsesTheMessageDPI(t *testing.T) {
	if got := panelScaleForDPI(144, 0); got != 1.5 {
		t.Fatalf("message DPI scale = %v, want 1.5", got)
	}
	if got := panelScaleForDPI(144, 2); got != 2 {
		t.Fatalf("capture DPI scale = %v, want 2", got)
	}
}

func TestDPIFrameCommitsOnlyTheLatestGeneration(t *testing.T) {
	p := panel{pendingDPI: 144, dpiGeneration: 3, dpiReadyGeneration: 2}
	if p.dpiFrameReady() {
		t.Fatal("stale DPI layout must remain cloaked")
	}
	p.dpiReadyGeneration = 3
	if !p.dpiFrameReady() {
		t.Fatal("latest DPI layout should be ready to present")
	}
}

func TestPositionPreservesScaledClientBounds(t *testing.T) {
	const (
		scale            = 1.5
		testClientHeight = 300
	)
	err := Capture(State{}, func(key string) string { return key }, scale, func(hwnd windows.Handle) error {
		p := panelFor(hwnd)
		if p == nil {
			t.Fatal("capture panel is not registered")
		}
		// Keep the real capture scale while using a height that fits the smaller
		// virtual desktop exposed by GitHub's Windows runners.
		p.clientH = testClientHeight
		if err := p.position(p.style, p.exStyle); err != nil {
			t.Fatal(err)
		}
		width, height, err := nativeform.ClientSize(hwnd)
		if err != nil {
			t.Fatal(err)
		}
		wantWidth := p.sc(p.metrics.style.Layout.PanelWidth)
		wantHeight := p.sc(p.clientH)
		if width != wantWidth || height != wantHeight {
			t.Fatalf("capture client = %dx%d, want %dx%d", width, height, wantWidth, wantHeight)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestVisualStateTokensKeepSpecifiedLogicalSizes(t *testing.T) {
	control := defaultPanelStyle.Control
	if control.FocusInset != 2 || control.FocusRingWidth != 2 {
		t.Fatalf("focus tokens = inset %d width %d, want 2/2", control.FocusInset, control.FocusRingWidth)
	}
	if control.ArrowWidth != 8 || control.ArrowHeight != 4 {
		t.Fatalf("disclosure tokens = arrow %dx%d, want 8x4", control.ArrowWidth, control.ArrowHeight)
	}
	metrics := newPanelMetrics(defaultPanelStyle, 1.5)
	if got := metrics.px(control.FocusRingWidth); got != 3 {
		t.Fatalf("scaled focus ring width = %d, want 3", got)
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

func TestUnavailableThemeDisablesEveryThemeControl(t *testing.T) {
	p := &panel{
		themeUnavailable: true,
		toggles:          map[uint16]bool{idTheme: true},
		disabled:         map[uint16]bool{},
		controls:         map[uint16]windows.Handle{},
		tooltips:         map[uint16][]uint16{},
	}
	p.applyDependentStates()
	for _, id := range []uint16{idTheme, idFullscreen, idBattery, idIPLocation, idThemeSwitch, idThemeRepair} {
		if !p.disabled[id] {
			t.Fatalf("theme control %d remained enabled", id)
		}
	}
}

func TestMenuAndChoiceTriggersIgnoreStaleNativeHotlight(t *testing.T) {
	p := &panel{
		toggles:  map[uint16]bool{},
		selected: map[uint16]bool{},
		disabled: map[uint16]bool{},
	}
	for _, id := range []uint16{idQuickActions, idLanguage, idIdleTimeout, idIdleAction} {
		if state := p.controlState(id, odsHotlight); state.Hovered {
			t.Fatalf("trigger %d retained native hotlight after mouse leave", id)
		}
	}
	p.hoverID = idLanguage
	if state := p.controlState(idLanguage, 0); !state.Hovered {
		t.Fatal("tracked hover must remain visible for language trigger")
	}
}

func TestChoiceTriggerKeyboardKeysOpenTheSharedPopup(t *testing.T) {
	for _, key := range []uintptr{vkReturn, vkSpace, vkUp, vkDown, vkF4} {
		if !isChoiceOpenKey(key) {
			t.Fatalf("key %#x should open a choice popup", key)
		}
	}
	for _, key := range []uintptr{vkEscape, vkHome, vkEnd} {
		if isChoiceOpenKey(key) {
			t.Fatalf("key %#x must not open a closed choice popup", key)
		}
	}
}

func TestChoiceTriggerTogglesARealSharedPopup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping native Win32 integration test in short mode")
	}
	err := Capture(State{IdleTimeout: 30}, func(key string) string { return key }, 1, func(hwnd windows.Handle) error {
		p := panelFor(hwnd)
		if p == nil {
			t.Fatal("capture panel is not active")
		}
		p.openChoice(idIdleTimeout)
		popup := p.choice.popup
		if p.choice.openID != idIdleTimeout || popup == nil || !popup.IsOpen() || popup.Window() == 0 {
			t.Fatal("choice trigger did not create the shared native popup")
		}
		p.openChoice(idIdleTimeout)
		if p.choice.openID != 0 || p.choice.popup != nil || popup.IsOpen() {
			t.Fatal("clicking the open choice trigger did not close its popup")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFixedMenuTriggersUseTheSharedPopup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping native Win32 integration test in short mode")
	}
	err := Capture(State{}, func(key string) string { return key }, 1, func(hwnd windows.Handle) error {
		p := panelFor(hwnd)
		if p == nil {
			t.Fatal("capture panel is not active")
		}
		for _, id := range append(quickActionIDs(), languageIDs()...) {
			if p.controls[id] != 0 {
				t.Fatalf("legacy fixed-menu child %d still exists", id)
			}
		}
		p.openQuickMenu()
		quick := p.choice.popup
		if p.choice.openID != idQuickActions || quick == nil || !quick.IsOpen() {
			t.Fatal("system controls did not open the shared popup")
		}
		p.openLanguageMenu()
		language := p.choice.popup
		if quick.IsOpen() || p.choice.openID != idLanguage || language == nil || !language.IsOpen() {
			t.Fatal("language settings did not replace the system-controls popup")
		}
		p.closeChoice(false)
		if language.IsOpen() || p.choice.openID != 0 || p.choice.popup != nil {
			t.Fatal("language popup did not close through the shared lifecycle")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPanelBackgroundClickClosesEveryOpenMenu(t *testing.T) {
	p := &panel{
		choice: choiceSurface{
			openID: idQuickActions,
		},
	}
	p.closeOpenMenus()
	if p.choice.openID != 0 {
		t.Fatalf("background click left popup open: %d", p.choice.openID)
	}
}

func TestMenuClickKeepsOnlyTheOpenSurfaceInteractive(t *testing.T) {
	p := &panel{choice: choiceSurface{openID: idQuickActions}}
	if !p.menuClickKeepsOpen(idQuickActions) {
		t.Fatal("clicking the open system-controls trigger should keep its popup alive until the deferred toggle")
	}
	if p.menuClickKeepsOpen(idSleep) {
		t.Fatal("popup values are no longer owner child controls")
	}
	if p.menuClickKeepsOpen(idLanguage) {
		t.Fatal("another trigger must close the current menu before switching")
	}

	p = &panel{choice: choiceSurface{openID: idIdleTimeout}}
	if !p.menuClickKeepsOpen(idIdleTimeout) {
		t.Fatal("clicking the open choice trigger should keep its popup alive until the deferred toggle")
	}
	if p.menuClickKeepsOpen(idIdleAction) {
		t.Fatal("another choice trigger must close the open choice before switching")
	}
}

func TestCommandActionMapsDirectCommands(t *testing.T) {
	p := &panel{}
	tests := []struct {
		id     uint16
		action Action
	}{
		{idSleep, ActSleep},
		{idHibernate, ActHibernate},
		{idShutdown, ActShutdown},
		{idLock, ActLock},
		{idRestart, ActRestart},
		{idThemeSwitch, ActSwitchTheme},
		{idThemeRepair, ActRepairTheme},
		{idConfig, ActConfig},
		{idProjectHome, ActProjectHome},
		{idExit, ActExit},
	}
	for _, test := range tests {
		action, value, ok := p.commandAction(test.id)
		if !ok || action != test.action || value != 0 {
			t.Fatalf("command %d = (%d, %d, %v), want (%d, 0, true)", test.id, action, value, ok, test.action)
		}
	}
}

func TestToggleCommandsPreserveIdleMutualExclusion(t *testing.T) {
	p := &panel{
		toggles:  map[uint16]bool{idIdle: true},
		disabled: map[uint16]bool{},
	}
	action, ok := p.toggleCommand(idNoSleep)
	if !ok || action != ActNoSleepToggle || !p.toggles[idNoSleep] || p.toggles[idIdle] {
		t.Fatalf("Stay Awake toggle = action %d, ok=%v, toggles=%v", action, ok, p.toggles)
	}

	p.toggles[idNoSleep] = true
	action, ok = p.toggleCommand(idIdle)
	if !ok || action != ActIdleToggle || !p.toggles[idIdle] || p.toggles[idNoSleep] {
		t.Fatalf("idle toggle = action %d, ok=%v, toggles=%v", action, ok, p.toggles)
	}
}

func TestLanguageCommandDispatchesOnlyForAChange(t *testing.T) {
	var actions []Action
	var values []int
	p := &panel{
		selected: map[uint16]bool{idLangEN: true},
		controls: map[uint16]windows.Handle{},
		onAction: func(action Action, value int) {
			actions = append(actions, action)
			values = append(values, value)
		},
	}
	p.selectLanguage(idLangEN, 0)
	p.selectLanguage(idLangZH, 1)
	if len(actions) != 1 || actions[0] != ActLanguage || values[0] != 1 {
		t.Fatalf("language actions = %v, values = %v", actions, values)
	}
}

func TestEveryControlPanelActionHasAUICommandPath(t *testing.T) {
	p := &panel{
		toggles:  map[uint16]bool{},
		disabled: map[uint16]bool{},
	}
	mapped := map[Action]bool{
		ActIdleTimeout: true,
		ActIdleAction:  true,
		ActLanguage:    true,
	}
	for _, id := range []uint16{
		idAutomation, idSleep, idHibernate, idShutdown, idLock, idRestart,
		idThemeSwitch, idThemeRepair, idConfig, idProjectHome, idExit,
	} {
		action, _, ok := p.commandAction(id)
		if !ok {
			t.Fatalf("command ID %d has no action", id)
		}
		mapped[action] = true
	}
	for _, id := range []uint16{
		idNoSleep, idAutomationEnabled, idIdle, idIdleWarning, idIdleEnhanced,
		idTheme, idBattery, idFullscreen, idIPLocation,
		idHotkeys, idAutostart, idLogging,
	} {
		action, ok := p.toggleCommand(id)
		if !ok {
			t.Fatalf("toggle ID %d has no action", id)
		}
		mapped[action] = true
	}

	for action := ActSleep; action <= ActIdleToggle; action++ {
		if !mapped[action] {
			t.Errorf("control panel action %d has no UI command path", action)
		}
	}
	for action := ActIdleTimeout; action <= ActExit; action++ {
		if !mapped[action] {
			t.Errorf("control panel action %d has no UI command path", action)
		}
	}
}
