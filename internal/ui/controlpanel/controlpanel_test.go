package controlpanel

import (
	"strings"
	"testing"

	"github.com/JeffioZ/idletrigger/internal/ui/colors"
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

func TestChoiceOptionOwnerAndSelectionModel(t *testing.T) {
	p := &panel{
		choice: choiceSurface{
			optionIDs: map[uint16][]uint16{
				idIdleTimeout: {idTimeoutOptionBase, idTimeoutOptionBase + 1},
				idIdleAction:  {idActionOptionBase, idActionOptionBase + 1},
			},
			selected: map[uint16]int{idIdleTimeout: 1, idIdleAction: 0},
		},
	}
	owner, index, ok := choiceOptionOwner(p, idTimeoutOptionBase+1)
	if !ok || owner != idIdleTimeout || index != 1 {
		t.Fatalf("timeout option lookup = (%d, %d, %v)", owner, index, ok)
	}
	if owner, _, ok := choiceOptionOwner(p, idActionOptionBase); !ok || owner != idIdleAction {
		t.Fatalf("action option lookup failed: owner=%d ok=%v", owner, ok)
	}
}

func TestChoiceOptionRangesDoNotOverlap(t *testing.T) {
	timeoutIDs := make(map[uint16]bool)
	for i := 0; i < len(timeoutMinutes); i++ {
		timeoutIDs[idTimeoutOptionBase+uint16(i)] = true
	}
	for i := 0; i < 4; i++ {
		if timeoutIDs[idActionOptionBase+uint16(i)] {
			t.Fatalf("action option ID %d overlaps timeout range", idActionOptionBase+uint16(i))
		}
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
	p.quickMenuOpen = true
	if !p.triggerOpen(idQuickActions) || p.triggerOpen(idLanguage) {
		t.Fatal("quick menu open state was not isolated")
	}
	p.quickMenuOpen, p.languageMenuOpen = false, true
	if p.triggerOpen(idQuickActions) || !p.triggerOpen(idLanguage) {
		t.Fatal("language menu open state was not isolated")
	}
	p.languageMenuOpen = false
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

func TestMenuOptionStylesKeepOnlySemanticDifferences(t *testing.T) {
	if got := menuOptionStyleFor(idLangEN, true); !got.Selected || got.Danger {
		t.Fatalf("selected language style = %+v", got)
	}
	if got := menuOptionStyleFor(idLock, false); got.Selected || got.Danger {
		t.Fatalf("regular quick action style = %+v", got)
	}
	if got := menuOptionStyleFor(idShutdown, false); got.Selected || !got.Danger {
		t.Fatalf("shutdown style = %+v", got)
	}
	if got := menuOptionStyleFor(idRestart, false); got.Selected || !got.Danger {
		t.Fatalf("restart style = %+v", got)
	}
}

func TestSharedMenuGeometryIncludesRowGaps(t *testing.T) {
	const rowHeight, rowGap, surfaceInset = 34, 1, 4
	heights := map[int]int{0: 8, 1: 42, 2: 77, 5: 182, 10: 357}
	for rows, want := range heights {
		if got := menuHeight(rows, rowHeight, rowGap, surfaceInset); got != want {
			t.Fatalf("menuHeight(%d) = %d, want %d", rows, got, want)
		}
	}
	offsets := map[int]int{0: 4, 1: 39, 4: 144, 9: 319}
	for index, want := range offsets {
		if got := menuRowOffset(index, rowHeight, rowGap, surfaceInset); got != want {
			t.Fatalf("menuRowOffset(%d) = %d, want %d", index, got, want)
		}
	}
}

func TestSharedMenuRowsFitExactAvailableHeight(t *testing.T) {
	const rowHeight, rowGap, surfaceInset = 34, 1, 4
	cases := []struct {
		available int
		want      int
	}{
		{41, 0},
		{42, 1},
		{76, 1},
		{77, 2},
		{181, 4},
		{182, 5},
	}
	for _, tc := range cases {
		if got := menuRowsFit(tc.available, rowHeight, rowGap, surfaceInset); got != tc.want {
			t.Fatalf("menuRowsFit(%d) = %d, want %d", tc.available, got, tc.want)
		}
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

func TestVisualStateTokensKeepSpecifiedLogicalSizes(t *testing.T) {
	control := defaultPanelStyle.Control
	if control.FocusInset != 2 || control.FocusRingWidth != 2 {
		t.Fatalf("focus tokens = inset %d width %d, want 2/2", control.FocusInset, control.FocusRingWidth)
	}
	if control.ArrowWidth != 8 || control.ArrowHeight != 4 || control.SelectedMarkerWidth != 3 {
		t.Fatalf("disclosure tokens = arrow %dx%d marker %d, want 8x4/3", control.ArrowWidth, control.ArrowHeight, control.SelectedMarkerWidth)
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

func TestClosingHoverMenuClearsTrackedHover(t *testing.T) {
	p := &panel{
		quickMenuOpen:    true,
		languageMenuOpen: true,
		hoverID:          idQuickActions,
		controls:         map[uint16]windows.Handle{},
	}
	p.closeQuickMenu()
	if p.quickMenuOpen || p.hoverID != 0 {
		t.Fatalf("quick menu close retained state: open=%v hover=%d", p.quickMenuOpen, p.hoverID)
	}
	p.hoverID = idLanguage
	p.closeLanguageMenu()
	if p.languageMenuOpen || p.hoverID != 0 {
		t.Fatalf("language menu close retained state: open=%v hover=%d", p.languageMenuOpen, p.hoverID)
	}
}

func TestChoiceOpenOnlyEntersFocusVisibleForKeyboard(t *testing.T) {
	mouse := &panel{choice: choiceSurface{scroll: map[uint16]int{}, selected: map[uint16]int{}}}
	if mouse.beginChoiceOpen(idIdleTimeout) || mouse.keyboardNavigation || mouse.choice.restoreFocus {
		t.Fatal("mouse-opened choice must not enable keyboard focus-visible")
	}
	keyboard := &panel{choice: choiceSurface{
		focusOnOpen: true,
		scroll:      map[uint16]int{},
		selected:    map[uint16]int{idIdleTimeout: 3},
	}}
	if !keyboard.beginChoiceOpen(idIdleTimeout) || !keyboard.keyboardNavigation || !keyboard.choice.restoreFocus {
		t.Fatal("keyboard-opened choice must preserve focus-visible restoration")
	}
	if got := keyboard.choice.scroll[idIdleTimeout]; got != 3 {
		t.Fatalf("keyboard choice scroll = %d, want selected option 3", got)
	}
}

func TestChoiceFocusContainsItsOpenTrigger(t *testing.T) {
	trigger := windows.Handle(17)
	p := &panel{
		controls: map[uint16]windows.Handle{idIdleTimeout: trigger},
		choice: choiceSurface{
			openID: idIdleTimeout,
		},
	}
	if !p.choiceFocusContains(trigger) {
		t.Fatal("the open choice trigger must retain focus while its menu is visible")
	}
}

func TestPanelBackgroundClickClosesEveryOpenMenu(t *testing.T) {
	p := &panel{
		quickMenuOpen:    true,
		languageMenuOpen: true,
		choice: choiceSurface{
			openID: idIdleTimeout,
		},
	}
	p.closeOpenMenus()
	if p.quickMenuOpen || p.languageMenuOpen || p.choice.openID != 0 {
		t.Fatalf("background click left menus open: quick=%v language=%v choice=%d", p.quickMenuOpen, p.languageMenuOpen, p.choice.openID)
	}
}

func TestMenuClickKeepsOnlyTheOpenSurfaceInteractive(t *testing.T) {
	p := &panel{quickMenuOpen: true}
	for _, id := range []uint16{idQuickActions, idQuickMenu, idSleep} {
		if !p.menuClickKeepsOpen(id) {
			t.Fatalf("quick menu click %d should keep the menu open", id)
		}
	}
	if p.menuClickKeepsOpen(idLanguage) {
		t.Fatal("another trigger must close the current menu before switching")
	}

	p = &panel{choice: choiceSurface{openID: idIdleTimeout, optionIDs: map[uint16][]uint16{idIdleTimeout: {idTimeoutOptionBase}}}}
	for _, id := range []uint16{idIdleTimeout, idChoiceSurface, idTimeoutOptionBase} {
		if !p.menuClickKeepsOpen(id) {
			t.Fatalf("choice click %d should keep the menu open", id)
		}
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
