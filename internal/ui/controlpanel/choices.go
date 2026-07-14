package controlpanel

import (
	"fmt"

	"github.com/JeffioZ/idletrigger/internal/config"
)

type timeoutChoice struct {
	minutes int
	label   string
}

// Keep the complete list short enough to fit in the popup without a special
// scrolling path. A persisted custom value remains active at runtime; only
// its popup presentation falls back to the default preset until custom input
// is added to the UI.
var timeoutMinutes = []int{1, 2, 3, 5, 10, 15, config.DefaultIdleTimeoutMinutes, 60, 120, 300}

func supportedTimeout(minutes int) int {
	for _, supported := range timeoutMinutes {
		if supported == minutes {
			return minutes
		}
	}
	return config.DefaultIdleTimeoutMinutes
}

func timeoutChoices(current int, chinese bool) ([]timeoutChoice, int) {
	current = supportedTimeout(current)
	choices := make([]timeoutChoice, 0, len(timeoutMinutes))
	selected := -1
	for _, minutes := range timeoutMinutes {
		choices = append(choices, timeoutChoice{minutes: minutes, label: formatTimeout(minutes, chinese)})
		if minutes == current {
			selected = len(choices) - 1
		}
	}
	if selected >= 0 {
		return choices, selected
	}
	return choices, timeoutIndex(choices, current)
}

func formatTimeout(minutes int, chinese bool) string {
	if minutes%60 == 0 {
		if chinese {
			return fmt.Sprintf("%d 小时", minutes/60)
		}
		if minutes == 60 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", minutes/60)
	}
	if chinese {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

func timeoutLabels(current int, chinese bool, options *[]timeoutChoice) []string {
	*options, _ = timeoutChoices(current, chinese)
	labels := make([]string, len(*options))
	for i, option := range *options {
		labels[i] = option.label
	}
	return labels
}

func timeoutIndex(options []timeoutChoice, current int) int {
	current = supportedTimeout(current)
	for i, option := range options {
		if option.minutes == current {
			return i
		}
	}
	return 0
}

func quickActionIDs() []uint16 {
	return []uint16{idLock, idSleep, idHibernate, idShutdown, idRestart}
}

func languageIDs() []uint16 { return []uint16{idLangEN, idLangZH} }

func actionIndex(value string) int {
	index := config.IdleActionIndex(config.Action(value))
	if index < 0 {
		return 0
	}
	return index
}

func actionLabels(p *panel) []string {
	return []string{p.text("menu_action_sleep"), p.text("menu_action_hibernate"), p.text("menu_action_shutdown"), p.text("menu_action_lock")}
}
