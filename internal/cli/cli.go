// Package cli implements the command-line interface for IdleTrigger.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/JeffioZ/idletrigger/internal/actions"
	"github.com/JeffioZ/idletrigger/internal/autostart"
	"github.com/JeffioZ/idletrigger/internal/i18n"
	"github.com/JeffioZ/idletrigger/internal/ipc"
	"github.com/JeffioZ/idletrigger/internal/monitor"
	"github.com/JeffioZ/idletrigger/internal/nosleep"
	"github.com/JeffioZ/idletrigger/internal/log"
	"github.com/JeffioZ/idletrigger/internal/power"
)

// Run dispatches the first CLI argument.
func Run(lang string) {
	if len(os.Args) >= 2 {
		log.Info("CLI command: %s", os.Args[1])
	}
	if len(os.Args) < 2 {
		fmt.Println(i18n.T(lang, "cli_usage"))
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "sleep":
		caps := power.GetCapabilities()
		if !caps.SleepAvailable {
			fmt.Fprintln(os.Stderr, "error: sleep is not available on this system")
			os.Exit(1)
		}
		fmt.Println(i18n.T(lang, "msg_sleeping"))
		exitOnErr(actions.Sleep())

	case "hibernate":
		caps := power.GetCapabilities()
		if !caps.HibernateAvailable {
			fmt.Fprintln(os.Stderr, "error: hibernate is not available on this system")
			os.Exit(1)
		}
		fmt.Println(i18n.T(lang, "msg_hibernating"))
		exitOnErr(actions.Hibernate())

	case "shutdown":
		fmt.Println(i18n.T(lang, "msg_shutting_down"))
		exitOnErr(actions.Shutdown())

	case "lock":
		fmt.Println(i18n.T(lang, "msg_locking"))
		exitOnErr(actions.Lock())

	case "nosleep":
		cmdNoSleep(lang)

	case "monitor":
		cmdMonitor(lang)

	case "autostart":
		cmdAutostart(lang)

	case "status":
		cmdStatus(lang)

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

func cmdNoSleep(lang string) {
	if len(os.Args) < 3 {
		fmt.Println("Usage: IdleTrigger nosleep <on|off|toggle|status>")
		os.Exit(1)
	}
	sub := os.Args[2]
	if ipc.IsTrayRunning() {
		ipcCmd := "nosleep:" + sub
		if sub == "on" && hasScreenFlag() {
			ipcCmd = "nosleep:on:screen"
		}
		if resp, ok := ipc.Send(ipcCmd); ok {
			fmt.Println(resp)
			return
		}
	}
	switch sub {
	case "on":
		nosleep.Enable(hasScreenFlag())
		fmt.Println(i18n.T(lang, "msg_nosleep_on"))
	case "off":
		nosleep.Disable()
		fmt.Println(i18n.T(lang, "msg_nosleep_off"))
	case "toggle":
		if nosleep.IsEnabled() {
			nosleep.Disable()
			fmt.Println(i18n.T(lang, "msg_nosleep_off"))
		} else {
			nosleep.Enable(hasScreenFlag())
			fmt.Println(i18n.T(lang, "msg_nosleep_on"))
		}
	case "status":
		printNoSleepStatus(lang)
	default:
		fmt.Println("Usage: IdleTrigger nosleep <on|off|toggle|status>")
		os.Exit(1)
	}
}

func cmdMonitor(lang string) {
	if len(os.Args) < 3 {
		fmt.Println("Usage: IdleTrigger monitor <on|off|status>")
		os.Exit(1)
	}
	if ipc.IsTrayRunning() {
		if resp, ok := ipc.Send("monitor:" + os.Args[2]); ok {
			fmt.Println(resp)
			return
		}
	}
	if os.Args[2] == "status" {
		printIdleStatus()
		return
	}
	fmt.Println("error: tray is not running; cannot control monitor directly")
	os.Exit(1)
}

func cmdAutostart(lang string) {
	if len(os.Args) < 3 {
		fmt.Println("Usage: IdleTrigger autostart <enable|disable|status>")
		return
	}
	switch os.Args[2] {
	case "enable":
		exitOnErr(autostart.Enable())
		fmt.Println(i18n.T(lang, "msg_autostart_enabled"))
	case "disable":
		exitOnErr(autostart.Disable())
		fmt.Println(i18n.T(lang, "msg_autostart_disabled"))
	case "status":
		enabled, err := autostart.IsEnabled()
		exitOnErr(err)
		fmt.Println("Auto-start:", onOff(enabled))
	default:
		fmt.Println("Usage: IdleTrigger autostart <enable|disable|status>")
	}
}

func cmdStatus(lang string) {
	if resp, ok := ipc.Send("status"); ok {
		fmt.Println(resp)
		return
	}
	fmt.Println("───────────────────────────")
	printNoSleepStatus(lang)
	printPowerStatus()
	printIdleStatus()
}

func printNoSleepStatus(lang string) {
	if nosleep.IsEnabled() {
		scr := ""
		if nosleep.IsKeepingScreenOn() {
			scr = " (keep screen on)"
		}
		fmt.Println("NoSleep:      ENABLED" + scr)
	} else {
		fmt.Println("NoSleep:      DISABLED")
	}
}

func printPowerStatus() {
	ps := power.GetStatus()
	if ps.ACLine {
		fmt.Println("Power:        AC")
	} else if ps.Battery {
		fmt.Printf("Power:        Battery %d%%\n", ps.Percent)
	} else {
		fmt.Println("Power:        Unknown")
	}
}

func printIdleStatus() {
	if d, err := monitor.IdleDuration(); err == nil {
		fmt.Printf("Idle time:    %s\n", d.Round(time.Second))
	} else {
		fmt.Println("Idle time:    unknown")
	}
}

func exitOnErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func hasScreenFlag() bool {
	for _, a := range os.Args {
		if a == "--screen" || a == "-s" {
			return true
		}
	}
	return false
}

func onOff(b bool) string {
	if b {
		return "ENABLED"
	}
	return "DISABLED"
}
