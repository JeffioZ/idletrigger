// Package automation defines the user-visible automatic-task model shared by
// configuration, runtime evaluation, and native UI. It deliberately contains
// data and pure validation only: automatic tasks can select built-in actions,
// but can never execute arbitrary commands or scripts.
package automation

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Action string

const (
	ActionStayAwake       Action = "stay_awake"
	ActionPauseStayAwake  Action = "pause_stay_awake"
	ActionEnableIdle      Action = "enable_idle_monitor"
	ActionPauseIdle       Action = "pause_idle_monitor"
	ActionLock            Action = "lock"
	ActionSleep           Action = "sleep"
	ActionHibernate       Action = "hibernate"
	ActionShutdown        Action = "shutdown"
	ActionRestart         Action = "restart"
	DefaultIdleMinutes           = 30
	DefaultWarningSeconds        = 60
	MinWarningSeconds            = 10
	MaxRules                     = 64
	MaxProcessesPerRule          = 64
)

type Trigger string

const (
	TriggerProcessRunning Trigger = "process_running"
	TriggerProcessStarted Trigger = "process_started"
	TriggerProcessExited  Trigger = "process_exited"
	TriggerTimeWindow     Trigger = "time_window"
	TriggerOnce           Trigger = "once"
	TriggerDaily          Trigger = "daily"
	TriggerWeekly         Trigger = "weekly"
)

type ProcessMatch string

const (
	MatchName ProcessMatch = "name"
	MatchPath ProcessMatch = "path"
)

type ProcessLogic string

const (
	ProcessAny  ProcessLogic = "any"
	ProcessAll  ProcessLogic = "all"
	ProcessNone ProcessLogic = "none"
)

type BlockedPolicy string

const (
	BlockedSkip BlockedPolicy = "skip"
	BlockedWait BlockedPolicy = "wait"
)

// ProcessTarget identifies an application, never one ephemeral PID. Name
// matching survives updates and moves; path matching intentionally stays exact.
type ProcessTarget struct {
	Match      ProcessMatch `toml:"match"`
	Executable string       `toml:"executable"`
	Path       string       `toml:"path,omitempty"`
}

func (p ProcessTarget) Key() string {
	if p.Match == MatchPath {
		return "path:" + strings.ToLower(filepath.Clean(p.Path))
	}
	return "name:" + strings.ToLower(p.Executable)
}

// Rule is one built-in automatic task. Internally state rules and event tasks
// are evaluated differently, while the UI presents one concise task concept.
type Rule struct {
	ID             string          `toml:"id"`
	Name           string          `toml:"name"`
	Enabled        bool            `toml:"enabled"`
	Action         Action          `toml:"action"`
	Trigger        Trigger         `toml:"trigger"`
	Time           string          `toml:"time,omitempty"`
	EndTime        string          `toml:"end_time,omitempty"`
	Date           string          `toml:"date,omitempty"`
	Days           []string        `toml:"days,omitempty"`
	ProcessLogic   ProcessLogic    `toml:"process_logic,omitempty"`
	Processes      []ProcessTarget `toml:"processes,omitempty"`
	KeepScreenOn   bool            `toml:"keep_screen_on,omitempty"`
	IdleMinutes    int             `toml:"idle_minutes,omitempty"`
	WarningSeconds int             `toml:"warning_seconds,omitempty"`
	BlockedPolicy  BlockedPolicy   `toml:"blocked_policy,omitempty"`
	MaxWaitMinutes int             `toml:"max_wait_minutes,omitempty"`
}

// RuleIssue describes why one configured rule is unsafe to run. Index is
// stable for the loaded rule list so callers can disable and annotate only the
// affected rule while continuing to use the others.
type RuleIssue struct {
	Index   int
	RuleID  string
	Message string
}

func (i RuleIssue) Error() string { return i.Message }

func IsStateAction(action Action) bool {
	switch action {
	case ActionStayAwake, ActionPauseStayAwake, ActionEnableIdle, ActionPauseIdle:
		return true
	default:
		return false
	}
}

func IsEventAction(action Action) bool { return ValidAction(action) && !IsStateAction(action) }

func ValidAction(action Action) bool {
	switch action {
	case ActionStayAwake, ActionPauseStayAwake, ActionEnableIdle, ActionPauseIdle,
		ActionLock, ActionSleep, ActionHibernate, ActionShutdown, ActionRestart:
		return true
	default:
		return false
	}
}

func ValidTrigger(trigger Trigger) bool {
	switch trigger {
	case TriggerProcessRunning, TriggerProcessStarted, TriggerProcessExited, TriggerTimeWindow,
		TriggerOnce, TriggerDaily, TriggerWeekly:
		return true
	default:
		return false
	}
}

func NormalizeRules(rules []Rule) []Rule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]Rule, 0, len(rules))
	seenIDs := make(map[string]struct{}, len(rules))
	for index, value := range rules {
		r := value
		r.ID = strings.TrimSpace(r.ID)
		if r.ID == "" {
			r.ID = fmt.Sprintf("rule-%d", index+1)
		}
		if _, exists := seenIDs[strings.ToLower(r.ID)]; exists {
			r.ID = fmt.Sprintf("%s-%d", r.ID, index+1)
		}
		seenIDs[strings.ToLower(r.ID)] = struct{}{}
		r.Name = strings.TrimSpace(r.Name)
		if r.Name == "" {
			r.Name = r.ID
		}
		if r.ProcessLogic != ProcessAny && r.ProcessLogic != ProcessAll && r.ProcessLogic != ProcessNone {
			r.ProcessLogic = ProcessAny
		}
		if r.BlockedPolicy != BlockedSkip && r.BlockedPolicy != BlockedWait {
			r.BlockedPolicy = BlockedSkip
		}
		if r.IdleMinutes <= 0 || r.IdleMinutes > 7*24*60 {
			r.IdleMinutes = DefaultIdleMinutes
		}
		if IsEventAction(r.Action) && (r.WarningSeconds < MinWarningSeconds || r.WarningSeconds > 3600) {
			r.WarningSeconds = DefaultWarningSeconds
		} else if r.WarningSeconds < 0 || r.WarningSeconds > 3600 {
			r.WarningSeconds = 0
		}
		if r.MaxWaitMinutes < 0 || r.MaxWaitMinutes > 7*24*60 {
			r.MaxWaitMinutes = 0
		}
		r.Days = normalizeDays(r.Days)
		// Older time-window rules used an empty day list to mean every day.
		// Store that state explicitly so the editor never shows "no days"
		// while the scheduler actually runs the rule every day.
		if r.Trigger == TriggerTimeWindow && len(r.Days) == 0 {
			r.Days = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
		}
		r.Processes = NormalizeTargets(r.Processes)
		out = append(out, r)
	}
	return out
}

func NormalizeTargets(targets []ProcessTarget) []ProcessTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]ProcessTarget, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	nameCovered := make(map[string]struct{}, len(targets))
	for _, value := range targets {
		t := value
		t.Executable = strings.TrimSpace(filepath.Base(t.Executable))
		t.Path = strings.TrimSpace(t.Path)
		if t.Match != MatchPath {
			t.Match = MatchName
			t.Path = ""
		}
		if t.Executable == "" || t.Executable == "." {
			continue
		}
		if t.Match == MatchPath {
			if !filepath.IsAbs(t.Path) {
				continue
			}
			t.Path = filepath.Clean(t.Path)
			if !strings.EqualFold(filepath.Base(t.Path), t.Executable) {
				continue
			}
		} else {
			nameCovered[strings.ToLower(t.Executable)] = struct{}{}
		}
		key := t.Key()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	// A name target already includes all exact paths with that executable.
	filtered := out[:0]
	for _, target := range out {
		if target.Match == MatchPath {
			if _, covered := nameCovered[strings.ToLower(target.Executable)]; covered {
				continue
			}
		}
		filtered = append(filtered, target)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if !strings.EqualFold(filtered[i].Executable, filtered[j].Executable) {
			return strings.ToLower(filtered[i].Executable) < strings.ToLower(filtered[j].Executable)
		}
		return filtered[i].Key() < filtered[j].Key()
	})
	return filtered
}

// PrepareRules is the single normalization and validation entry point used by
// configuration loading, the editor, and the runner. It never truncates input.
// Invalid rules remain present and receive one deterministic diagnostic.
func PrepareRules(rules []Rule) ([]Rule, []RuleIssue) {
	normalized := NormalizeRules(rules)
	issues := make([]RuleIssue, 0)
	addIssue := func(index int, message string) {
		ruleID := ""
		if index >= 0 && index < len(normalized) {
			ruleID = normalized[index].ID
		}
		issues = append(issues, RuleIssue{Index: index, RuleID: ruleID, Message: message})
	}

	seen := make(map[string]struct{}, len(rules))
	for index, raw := range rules {
		if index >= MaxRules {
			addIssue(index, fmt.Sprintf("automation_rules may contain at most %d rules", MaxRules))
			continue
		}
		id := strings.TrimSpace(raw.ID)
		if id == "" {
			addIssue(index, fmt.Sprintf("automation_rules[%d].id is required", index))
			continue
		}
		key := strings.ToLower(id)
		if _, exists := seen[key]; exists {
			addIssue(index, fmt.Sprintf("duplicate automation rule id %q", id))
			continue
		}
		seen[key] = struct{}{}
		if err := validatePreparedRule(raw, normalized[index]); err != nil {
			addIssue(index, fmt.Sprintf("automation rule %q: %v", id, err))
		}
	}
	return normalized, issues
}

func validatePreparedRule(raw, normalized Rule) error {
	if !ValidAction(raw.Action) {
		return fmt.Errorf("invalid action %q", raw.Action)
	}
	if !ValidTrigger(raw.Trigger) {
		return fmt.Errorf("invalid trigger %q", raw.Trigger)
	}
	if raw.ProcessLogic != "" && raw.ProcessLogic != ProcessAny && raw.ProcessLogic != ProcessAll && raw.ProcessLogic != ProcessNone {
		return fmt.Errorf("invalid process_logic %q", raw.ProcessLogic)
	}
	if raw.BlockedPolicy != "" && raw.BlockedPolicy != BlockedSkip && raw.BlockedPolicy != BlockedWait {
		return fmt.Errorf("invalid blocked_policy %q", raw.BlockedPolicy)
	}
	if raw.IdleMinutes < 0 || raw.IdleMinutes > 7*24*60 || (raw.Action == ActionEnableIdle && raw.IdleMinutes == 0) {
		return fmt.Errorf("idle_minutes must be between 1 and 10080")
	}
	if raw.WarningSeconds < 0 || raw.WarningSeconds > 3600 || (IsEventAction(raw.Action) && raw.WarningSeconds < MinWarningSeconds) {
		return fmt.Errorf("warning_seconds must be between %d and 3600", MinWarningSeconds)
	}
	if raw.MaxWaitMinutes < 0 || raw.MaxWaitMinutes > 7*24*60 {
		return fmt.Errorf("max_wait_minutes must be between 0 and 10080")
	}
	if IsEventAction(raw.Action) && len(raw.Processes) > 0 && raw.Trigger != TriggerProcessStarted && raw.Trigger != TriggerProcessExited && raw.BlockedPolicy == BlockedWait && raw.MaxWaitMinutes == 0 {
		return fmt.Errorf("max_wait_minutes must be between 1 and 10080 when waiting")
	}
	if len(raw.Processes) > MaxProcessesPerRule {
		return fmt.Errorf("may contain at most %d process targets", MaxProcessesPerRule)
	}
	if hasInvalidDay(raw.Days) {
		return fmt.Errorf("contains an invalid weekday")
	}
	for _, target := range raw.Processes {
		if err := validateProcessTarget(target); err != nil {
			return fmt.Errorf("invalid process target: %v", err)
		}
	}
	if err := validateActionTrigger(normalized); err != nil {
		return err
	}
	return nil
}

// RuntimeRules returns a detached rule list with only invalid entries disabled.
// It preserves valid entries even when another rule in the same TOML is bad.
func RuntimeRules(rules []Rule, issues []RuleIssue) []Rule {
	out := append([]Rule(nil), rules...)
	for _, issue := range issues {
		if issue.Index >= 0 && issue.Index < len(out) {
			out[issue.Index].Enabled = false
		}
	}
	return out
}

func ValidateRules(rules []Rule) error {
	_, issues := PrepareRules(rules)
	if len(issues) > 0 {
		return issues[0]
	}
	return nil
}

func validateProcessTarget(target ProcessTarget) error {
	if target.Match != "" && target.Match != MatchName && target.Match != MatchPath {
		return fmt.Errorf("invalid match %q", target.Match)
	}
	executable := strings.TrimSpace(filepath.Base(target.Executable))
	if executable == "" || executable == "." {
		return fmt.Errorf("executable is required")
	}
	if target.Match == MatchPath {
		path := strings.TrimSpace(target.Path)
		if !filepath.IsAbs(path) {
			return fmt.Errorf("path must be absolute")
		}
		if !strings.EqualFold(filepath.Base(path), executable) {
			return fmt.Errorf("path does not match executable")
		}
	}
	return nil
}

func hasInvalidDay(days []string) bool {
	for _, value := range days {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "sun", "mon", "tue", "wed", "thu", "fri", "sat":
		default:
			return true
		}
	}
	return false
}

func validateActionTrigger(r Rule) error {
	if IsStateAction(r.Action) {
		if r.Trigger != TriggerProcessRunning && r.Trigger != TriggerTimeWindow {
			return fmt.Errorf("state action %q requires process_running or time_window", r.Action)
		}
	} else if r.Trigger == TriggerProcessRunning || r.Trigger == TriggerTimeWindow {
		return fmt.Errorf("event action %q requires once, daily, weekly, process_started, or process_exited", r.Action)
	}
	if (r.Trigger == TriggerProcessRunning || r.Trigger == TriggerProcessStarted || r.Trigger == TriggerProcessExited) && len(r.Processes) == 0 {
		return fmt.Errorf("trigger %q requires at least one process", r.Trigger)
	}
	if r.Trigger == TriggerOnce {
		if _, err := time.Parse("2006-01-02", r.Date); err != nil {
			return fmt.Errorf("invalid date %q", r.Date)
		}
	}
	if r.Trigger == TriggerOnce || r.Trigger == TriggerDaily || r.Trigger == TriggerWeekly || r.Trigger == TriggerTimeWindow {
		if _, err := time.Parse("15:04", r.Time); err != nil {
			return fmt.Errorf("invalid time %q", r.Time)
		}
	}
	if r.Trigger == TriggerTimeWindow {
		if _, err := time.Parse("15:04", r.EndTime); err != nil {
			return fmt.Errorf("invalid end_time %q", r.EndTime)
		}
	}
	if (r.Trigger == TriggerWeekly || r.Trigger == TriggerTimeWindow) && len(normalizeDays(r.Days)) == 0 {
		return fmt.Errorf("weekly and time-window triggers require at least one day")
	}
	return nil
}

func normalizeDays(days []string) []string {
	order := map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	seen := make(map[string]struct{}, len(days))
	out := make([]string, 0, len(days))
	for _, value := range days {
		day := strings.ToLower(strings.TrimSpace(value))
		if _, ok := order[day]; !ok {
			continue
		}
		if _, ok := seen[day]; ok {
			continue
		}
		seen[day] = struct{}{}
		out = append(out, day)
	}
	sort.Slice(out, func(i, j int) bool { return order[out[i]] < order[out[j]] })
	return out
}

func WeekdayKey(day time.Weekday) string {
	return [...]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}[day]
}
