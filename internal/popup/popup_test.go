package popup

import "testing"

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
		{name: "choice selected", id: idActionLock, choiceSelected: true, wantRole: buttonChoice, wantActive: true},
		{name: "choice not selected", id: idLangEN, wantRole: buttonChoice, wantActive: false},
		{name: "command remains stateless", id: idRestart, toggleOn: true, choiceSelected: true, wantRole: buttonCommand, wantActive: false},
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

func TestButtonRoleMappingCoversEveryPanelAction(t *testing.T) {
	for _, id := range []uint16{idNoSleep, idProcess, idIdle, idIdleWarning, idIdleKeepalive, idTheme, idBattery, idFullscreen, idIPLocation, idHotkeys, idAutostart, idLogging} {
		if got := roleForButton(id); got != buttonToggle {
			t.Fatalf("toggle id %d has role %d", id, got)
		}
	}
	for _, id := range append(actionIDs(), languageIDs()...) {
		if got := roleForButton(id); got != buttonChoice {
			t.Fatalf("choice id %d has role %d", id, got)
		}
	}
	for _, id := range []uint16{idLock, idSleep, idHibernate, idShutdown, idRestart, idThemeSwitch, idThemeRepair, idConfig, idExit, idTestWarning} {
		if got := roleForButton(id); got != buttonCommand {
			t.Fatalf("command id %d has role %d", id, got)
		}
	}
}

func TestFocusOutlineKeepsSelectedButtonsContrasted(t *testing.T) {
	if focusOutlineUsesSurface(false) {
		t.Fatal("inactive button should keep the accent focus outline")
	}
	if !focusOutlineUsesSurface(true) {
		t.Fatal("active button should use the surface focus outline")
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
