package automation

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestNormalizeTargetsUsesStableApplicationIdentity(t *testing.T) {
	path := filepath.Join(`C:\Program Files`, "Example", "player.exe")
	targets := NormalizeTargets([]ProcessTarget{
		{Match: MatchPath, Executable: "player.exe", Path: path},
		{Match: MatchName, Executable: "PLAYER.EXE"},
		{Match: MatchName, Executable: "player.exe"},
	})
	if len(targets) != 1 || targets[0].Match != MatchName || targets[0].Executable != "PLAYER.EXE" {
		t.Fatalf("normalized targets = %+v", targets)
	}
}

func TestValidateRulesRejectsArbitraryOrUnsafeCombinations(t *testing.T) {
	target := ProcessTarget{Match: MatchName, Executable: "render.exe"}
	for _, rule := range []Rule{
		{ID: "unknown", Enabled: true, Action: Action("run_command"), Trigger: TriggerOnce, Date: "2026-07-15", Time: "12:00"},
		{ID: "power-while-running", Enabled: true, Action: ActionShutdown, Trigger: TriggerProcessRunning, Processes: []ProcessTarget{target}, WarningSeconds: 60},
		{ID: "awake-once", Enabled: true, Action: ActionStayAwake, Trigger: TriggerOnce, Date: "2026-07-15", Time: "12:00"},
		{ID: "silent-power", Enabled: true, Action: ActionRestart, Trigger: TriggerDaily, Time: "03:00", WarningSeconds: 0},
	} {
		if err := ValidateRules([]Rule{rule}); err == nil {
			t.Fatalf("unsafe or unsupported rule accepted: %+v", rule)
		}
	}
}

func TestValidateRulesAcceptsProcessAndScheduledBuiltIns(t *testing.T) {
	rules := NormalizeRules([]Rule{
		{ID: "awake", Enabled: true, Action: ActionStayAwake, Trigger: TriggerProcessRunning, Processes: []ProcessTarget{{Match: MatchName, Executable: "render.exe"}}},
		{ID: "lock-on-start", Enabled: true, Action: ActionLock, Trigger: TriggerProcessStarted, Processes: []ProcessTarget{{Match: MatchName, Executable: "render.exe"}}, WarningSeconds: 60},
		{ID: "restart", Enabled: true, Action: ActionRestart, Trigger: TriggerDaily, Time: "03:30", WarningSeconds: 60},
	})
	if err := ValidateRules(rules); err != nil {
		t.Fatalf("valid rules rejected: %v", err)
	}
}

func TestNormalizeTimeWindowMakesEveryDayExplicit(t *testing.T) {
	rules := NormalizeRules([]Rule{{
		ID: "work", Enabled: true, Action: ActionStayAwake,
		Trigger: TriggerTimeWindow, Time: "09:00", EndTime: "17:00",
	}})
	if len(rules) != 1 || len(rules[0].Days) != 7 {
		t.Fatalf("time-window days = %v, want all seven days", rules[0].Days)
	}
	if err := ValidateRules(rules); err != nil {
		t.Fatalf("normalized every-day time window rejected: %v", err)
	}
}

func TestPauseStayAwakeIsAStateAction(t *testing.T) {
	if !IsStateAction(ActionPauseStayAwake) || !ValidAction(ActionPauseStayAwake) {
		t.Fatal("pause Stay Awake must remain a built-in state action")
	}
	rules := NormalizeRules([]Rule{{
		ID: "pause-awake", Enabled: true, Action: ActionPauseStayAwake,
		Trigger: TriggerTimeWindow, Time: "09:00", EndTime: "17:00",
	}})
	if err := ValidateRules(rules); err != nil {
		t.Fatalf("pause Stay Awake rule rejected: %v", err)
	}
}

func TestNormalizeTargetsNeverSilentlyTruncates(t *testing.T) {
	targets := make([]ProcessTarget, 0, MaxProcessesPerRule+1)
	for index := 0; index <= MaxProcessesPerRule; index++ {
		targets = append(targets, ProcessTarget{Match: MatchName, Executable: fmt.Sprintf("process-%02d.exe", index)})
	}
	if got := NormalizeTargets(targets); len(got) != len(targets) {
		t.Fatalf("NormalizeTargets returned %d targets, want %d", len(got), len(targets))
	}
	_, issues := PrepareRules([]Rule{{
		ID: "too-many", Enabled: true, Action: ActionStayAwake, Trigger: TriggerProcessRunning,
		Processes: targets,
	}})
	if len(issues) != 1 || issues[0].Index != 0 {
		t.Fatalf("too many targets issues = %+v", issues)
	}
}

func TestPrepareRulesDisablesOnlyInvalidRuntimeEntries(t *testing.T) {
	rules, issues := PrepareRules([]Rule{
		{ID: "valid", Enabled: true, Action: ActionLock, Trigger: TriggerDaily, Time: "12:00", WarningSeconds: 60},
		{ID: "invalid", Enabled: true, Action: ActionLock, Trigger: TriggerDaily, Time: "12:00", WarningSeconds: 5},
	})
	if len(issues) != 1 || issues[0].Index != 1 {
		t.Fatalf("issues = %+v", issues)
	}
	runtimeRules := RuntimeRules(rules, issues)
	if !runtimeRules[0].Enabled || runtimeRules[1].Enabled {
		t.Fatalf("runtime enabled state = [%v %v]", runtimeRules[0].Enabled, runtimeRules[1].Enabled)
	}
}
