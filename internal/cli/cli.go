// Package cli implements the command-line interface for IdleTrigger.
package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/JeffioZ/idletrigger/internal/actions"
	"github.com/JeffioZ/idletrigger/internal/autostart"
	"github.com/JeffioZ/idletrigger/internal/i18n"
	"github.com/JeffioZ/idletrigger/internal/ipc"
	"github.com/JeffioZ/idletrigger/internal/monitor"
	"github.com/JeffioZ/idletrigger/internal/nosleep"
	"github.com/JeffioZ/idletrigger/internal/power"
)

// Run dispatches the first CLI argument.
func Run(lang string) {
	if len(os.Args) < 2 {
		fmt.Println(i18n.T(lang, "cli_usage"))
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "sleep":
		caps := power.GetCapabilities()
		if !caps.SleepAvailable {
			printError(lang, i18n.T(lang, "cli_error_sleep_unavailable"))
			os.Exit(1)
		}
		fmt.Println(i18n.T(lang, "msg_sleeping"))
		exitOnErr(lang, actions.Sleep())

	case "hibernate":
		caps := power.GetCapabilities()
		if !caps.HibernateAvailable {
			printError(lang, i18n.T(lang, "cli_error_hibernate_unavailable"))
			os.Exit(1)
		}
		fmt.Println(i18n.T(lang, "msg_hibernating"))
		exitOnErr(lang, actions.Hibernate())

	case "shutdown":
		fmt.Println(i18n.T(lang, "msg_shutting_down"))
		exitOnErr(lang, actions.Shutdown())

	case "restart":
		fmt.Println(i18n.T(lang, "msg_restarting"))
		exitOnErr(lang, actions.Restart())

	case "lock":
		fmt.Println(i18n.T(lang, "msg_locking"))
		exitOnErr(lang, actions.Lock())

	case "nosleep":
		cmdNoSleep(lang)

	case "monitor":
		cmdMonitor(lang)

	case "autostart":
		cmdAutostart(lang)

	case "status":
		cmdStatus(lang)

	case "diagnostics":
		cmdDiagnostics(lang)

	case "config:reload":
		if resp, ok := ipc.Send("config:reload"); ok {
			printIPCResponse(lang, resp)
		} else {
			printError(lang, i18n.T(lang, "cli_error_tray_not_running"))
			os.Exit(1)
		}

	case "version", "--version", "-V":
		fmt.Println(i18n.T(lang, "version"))

	case "help", "--help", "-h":
		fmt.Println(i18n.T(lang, "cli_usage"))

	default:
		fmt.Println(i18n.T(lang, "cli_unknown"))
		fmt.Println(i18n.T(lang, "cli_usage"))
		os.Exit(1)
	}
}

func cmdDiagnostics(lang string) {
	if len(os.Args) < 3 || os.Args[2] != "idle" {
		fmt.Fprintln(os.Stderr, "Usage: IdleTrigger diagnostics idle [--watch]")
		os.Exit(1)
	}
	watch := false
	for _, arg := range os.Args[3:] {
		if arg == "--watch" || arg == "-w" {
			watch = true
		}
	}
	for {
		snap, err := monitor.Snapshot()
		if err != nil {
			printError(lang, fmt.Sprintf("idle diagnostics failed: %v", err))
			os.Exit(1)
		}
		fmt.Printf("idle_diagnostics tick_now=%d tick32=%d last_input=%d raw_delta_ms=%d idle=%s\n",
			snap.NowTick64, snap.NowTick32, snap.LastInputTick, snap.RawDeltaMS, snap.Idle.Round(time.Millisecond))
		if !watch {
			return
		}
		time.Sleep(time.Second)
	}
}

func cmdNoSleep(lang string) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli_usage_nosleep"))
		os.Exit(1)
	}
	sub := os.Args[2]
	ipcCmd := "nosleep:" + sub
	if sub == "on" && hasScreenFlag() {
		ipcCmd = "nosleep:on:screen"
	}
	if resp, ok := ipc.Send(ipcCmd); ok {
		printIPCResponse(lang, resp)
		return
	}
	switch sub {
	case "status":
		printNoSleepStatus(lang)
	case "on", "off", "toggle":
		printError(lang, i18n.T(lang, "cli_error_nosleep_requires_tray"))
		os.Exit(1)
	default:
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli_usage_nosleep"))
		os.Exit(1)
	}
}

func cmdMonitor(lang string) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli_usage_monitor"))
		os.Exit(1)
	}
	if resp, ok := ipc.Send("monitor:" + os.Args[2]); ok {
		printIPCResponse(lang, resp)
		return
	}
	if os.Args[2] == "status" {
		fmt.Println(statusLine(lang, "status_monitor", i18n.T(lang, "status_not_running")))
		return
	}
	printError(lang, i18n.T(lang, "cli_error_monitor_requires_tray"))
	os.Exit(1)
}

func cmdAutostart(lang string) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli_usage_autostart"))
		os.Exit(1)
	}
	switch os.Args[2] {
	case "enable":
		exitOnErr(lang, autostart.Enable())
		fmt.Println(i18n.T(lang, "msg_autostart_enabled"))
	case "disable":
		exitOnErr(lang, autostart.Disable())
		fmt.Println(i18n.T(lang, "msg_autostart_disabled"))
	case "status":
		enabled, err := autostart.IsEnabled()
		exitOnErr(lang, err)
		fmt.Println(statusLine(lang, "menu_autostart", onOff(lang, enabled)))
	default:
		fmt.Fprintln(os.Stderr, i18n.T(lang, "cli_usage_autostart"))
		os.Exit(1)
	}
}

func cmdStatus(lang string) {
	if resp, ok := ipc.Send("status"); ok {
		printIPCResponse(lang, resp)
		return
	}
	fmt.Println(statusLine(lang, "status_tray", i18n.T(lang, "status_not_running")))
	printNoSleepStatus(lang)
	fmt.Println(statusLine(lang, "status_monitor", i18n.T(lang, "status_not_running")))
	printPowerStatus(lang)
	printIdleStatus(lang)
	fmt.Println(statusLine(lang, "status_hotkeys", i18n.T(lang, "status_disabled")))
}

func printIPCResponse(lang, resp string) {
	if strings.HasPrefix(resp, "err:") {
		printError(lang, strings.TrimSpace(strings.TrimPrefix(resp, "err:")))
		os.Exit(1)
	}
	if resp == "ok" {
		fmt.Println(i18n.T(lang, "cli_success"))
		return
	}
	fmt.Println(resp)
}

func printNoSleepStatus(lang string) {
	if nosleep.IsEnabled() {
		value := i18n.T(lang, "status_enabled")
		if nosleep.IsKeepingScreenOn() {
			value = i18n.T(lang, "status_enabled_keep_screen")
		}
		fmt.Println(statusLine(lang, "status_nosleep", value))
	} else {
		fmt.Println(statusLine(lang, "status_nosleep", i18n.T(lang, "status_disabled")))
	}
}

func printPowerStatus(lang string) {
	ps := power.GetStatus()
	value := i18n.T(lang, "status_unknown")
	if ps.Valid && ps.ACLine {
		value = i18n.T(lang, "status_ac_power")
	} else if ps.Valid && ps.Battery && ps.Percent >= 0 {
		value = fmt.Sprintf(i18n.T(lang, "status_battery"), ps.Percent)
	}
	fmt.Println(statusLine(lang, "status_power", value))
}

func printIdleStatus(lang string) {
	value := i18n.T(lang, "status_unknown")
	if d, err := monitor.IdleDuration(); err == nil {
		value = i18n.FormatDuration(lang, d)
	}
	fmt.Println(statusLine(lang, "status_idle_time", value))
}

func exitOnErr(lang string, err error) {
	if err != nil {
		printError(lang, err.Error())
		os.Exit(1)
	}
}

func printError(lang, detail string) {
	fmt.Fprintln(os.Stderr, fmt.Sprintf(i18n.T(lang, "cli_error_detail"), detail))
}

func statusLine(lang, labelKey, value string) string {
	return fmt.Sprintf(i18n.T(lang, "status_line"), i18n.T(lang, labelKey), value)
}

func hasScreenFlag() bool {
	for _, a := range os.Args {
		if a == "--screen" || a == "-s" {
			return true
		}
	}
	return false
}

func onOff(lang string, b bool) string {
	if b {
		return i18n.T(lang, "status_enabled")
	}
	return i18n.T(lang, "status_disabled")
}
