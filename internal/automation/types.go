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
	if len(rules) > MaxRules {
		rules = rules[:MaxRules]
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
		if !ValidAction(r.Action) || !ValidTrigger(r.Trigger) {
			r.Enabled = false
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
	if len(targets) > MaxProcessesPerRule {
		targets = targets[:MaxProcessesPerRule]
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

func ValidateRules(rules []Rule) error {
	if len(rules) > MaxRules {
		return fmt.Errorf("automation_rules may contain at most %d rules", MaxRules)
	}
	seen := make(map[string]struct{}, len(rules))
	for index, r := range rules {
		if strings.TrimSpace(r.ID) == "" {
			return fmt.Errorf("automation_rules[%d].id is required", index)
		}
		key := strings.ToLower(r.ID)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate automation rule id %q", r.ID)
		}
		seen[key] = struct{}{}
		if !ValidAction(r.Action) {
			return fmt.Errorf("automation rule %q has invalid action %q", r.ID, r.Action)
		}
		if !ValidTrigger(r.Trigger) {
			return fmt.Errorf("automation rule %q has invalid trigger %q", r.ID, r.Trigger)
		}
		if err := validateActionTrigger(r); err != nil {
			return fmt.Errorf("automation rule %q: %w", r.ID, err)
		}
		if IsEventAction(r.Action) && (r.WarningSeconds < MinWarningSeconds || r.WarningSeconds > 3600) {
			return fmt.Errorf("automation rule %q warning_seconds must be between %d and 3600", r.ID, MinWarningSeconds)
		}
		if len(r.Processes) > MaxProcessesPerRule {
			return fmt.Errorf("automation rule %q has too many process targets", r.ID)
		}
		for _, target := range r.Processes {
			if len(NormalizeTargets([]ProcessTarget{target})) != 1 {
				return fmt.Errorf("automation rule %q has an invalid process target", r.ID)
			}
		}
	}
	return nil
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
