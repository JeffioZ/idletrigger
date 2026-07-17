# IdleTrigger

[简体中文](README.zh-CN.md)

**A lightweight Windows tray utility for idle actions, automatic tasks, stay-awake control, and scheduled theme switching.**

IdleTrigger is a single executable with no runtime dependencies beyond Windows system DLLs. It stays out of the way in the notification area and keeps its settings in a readable TOML file beside the executable.

**[Download x64 for most PCs](https://github.com/JeffioZ/idletrigger/releases/latest/download/IdleTrigger-x64.exe)** · [Download x86 for 32-bit Windows](https://github.com/JeffioZ/idletrigger/releases/latest/download/IdleTrigger-x86.exe) · [Release notes and checksums](https://github.com/JeffioZ/idletrigger/releases/latest)

## When IdleTrigger Helps

- Keep downloads, renders, backups, or remote sessions awake without changing your usual Windows sleep settings.
- Lock, sleep, hibernate, or shut down after real keyboard and mouse inactivity.
- Enable or pause Stay Awake and Idle Monitoring automatically while selected apps are running or during a time window.
- Switch Windows light and dark themes on a schedule or around sunrise and sunset, with optional battery and fullscreen-aware behavior.

## How It Differs

Windows power settings are best for persistent display and sleep timeouts. [PowerToys Awake](https://learn.microsoft.com/windows/powertoys/awake) focuses on temporarily keeping a PC awake. IdleTrigger combines stay-awake control with actions after real keyboard and mouse inactivity, plus time- and process-based automation, in one portable tray app.

It uses Windows power requests and the system's last-input time instead of simulating mouse or keyboard activity.

## Requirements

- Windows 10 / Windows Server 2016 or later
- x64 build for most PCs; x86 build for 32-bit Windows
- Theme switching requires writable Windows Personalize light/dark settings. IdleTrigger detects unavailable or policy-restricted profiles and disables the entire Day/Night section without discarding its saved configuration.

Windows 7 is intentionally not supported by the main build. See [development guide](docs/development.md) for the compatibility rationale.

## Quick Start

1. Create a writable folder you intend to keep, such as `%LOCALAPPDATA%\IdleTrigger`.
2. Download the x64 build above for most PCs, or x86 only for 32-bit Windows, and place the EXE in that folder.
3. Run it. The app appears in the notification area without opening a main window.
4. Left-click the tray icon to open or close the control panel. Right-click it for the native **Open** and **Exit** menu.
5. Use the panel for common settings; edit `IdleTrigger.toml` beside the EXE for advanced settings.

The control panel follows Windows light/dark mode, responds to DPI changes, and stays open until you close it or left-click the tray icon again. Tooltips explain each available option.

### Updating or Moving

Exit IdleTrigger before replacing its EXE. Keep `IdleTrigger.toml` and `IdleTrigger.state.json` beside the EXE to preserve settings and automatic-task state. If you move the app, move those files together and launch it once from the new location before relying on automatic startup.

## Using the Control Panel

- **Power Management** contains Stay Awake and Idle Monitoring. Their toggles show manual settings; automatic tasks may change the current runtime state without rewriting those settings.
- **Automatic Tasks** shows the enabled count and next scheduled time. You can pause all tasks or manage individual rules and process conditions without editing TOML.
- **System Controls** run immediately, so save your work before choosing Sleep, Hibernate, Shut Down, or Restart.
- The panel supports mouse and keyboard navigation. Use `Tab` / `Shift+Tab` to move and `Space` to activate the focused control.

## Screenshots

<img src="docs/images/control-panel-en-light.png" alt="IdleTrigger control panel in English light mode" width="420">

<details>
<summary>More themes and languages</summary>

| English dark | Simplified Chinese light | Simplified Chinese dark |
| --- | --- | --- |
| <img src="docs/images/control-panel-en-dark.png" alt="IdleTrigger control panel in English dark mode" width="260"> | <img src="docs/images/control-panel-zh-CN-light.png" alt="IdleTrigger control panel in Simplified Chinese light mode" width="260"> | <img src="docs/images/control-panel-zh-CN-dark.png" alt="IdleTrigger control panel in Simplified Chinese dark mode" width="260"> |

</details>

## Idle Monitoring

Idle Monitoring is enabled by default with a 30-minute idle time and Sleep as its action. Available panel idle-time choices are:

`1, 2, 3, 5, 10, 15, 30 minutes; 1, 2, 5 hours`.

The monitor uses Windows `GetLastInputInfo` to observe real keyboard and mouse activity. Starting or re-enabling it begins a fresh idle window, so pre-existing idle time never triggers an immediate action. The timer resets after each action.

The **Enable Pre-action Reminder** switch shows a non-activating prompt before the action. Any keyboard or mouse input cancels the pending action; closing the prompt does the same. Set `idle_warning_seconds = 0` for silent operation.

If a device, driver, or app repeatedly resets Windows idle time and prevents idle actions, try **Enable Enhanced Monitoring**. It learns stable reset patterns while continuing to treat normal keyboard and mouse input as activity.

## Automatic Tasks

Use **Manage Automatic Tasks** to create rules without editing TOML. A rule can temporarily enable or pause Stay Awake or Idle Monitoring while selected processes are running or during a time window. It can also lock, sleep, hibernate, shut down, or restart once, on a daily or weekly schedule, when a selected process starts, or after all selected processes exit.

Processes can be matched by executable name or by a specific EXE. State is sampled every five seconds: processes already running when IdleTrigger starts do not backfill a start event, and a process that starts and exits between samples may be missed. Exit rules wait for every matching instance to close and use a five-second grace period to avoid firing during brief restarts.

System actions always show a cancellable countdown of at least 10 seconds. Tasks work only while IdleTrigger is running, and schedules missed during sleep are not replayed after resume. Rules can use built-in actions only; they cannot run custom commands or launch programs. Process matching does not inject code, terminate processes, install a service, or create Windows Task Scheduler entries.

## Command Line

Run the EXE without arguments to launch the tray app.

```text
IdleTrigger sleep | hibernate | shutdown | restart | lock

IdleTrigger nosleep on [--screen]
IdleTrigger nosleep off | toggle | status

IdleTrigger monitor on | off | status

IdleTrigger autostart enable | disable | status
IdleTrigger config:reload
IdleTrigger status
IdleTrigger version
```

Commands that change `nosleep` or `monitor` state, plus `config:reload`, require the tray app to be running. Status queries and one-shot power actions also work without it.

## Configuration

IdleTrigger creates and maintains `IdleTrigger.toml` next to the EXE while preserving valid existing values. Automatic rules are stored there; scheduler state is kept separately in `IdleTrigger.state.json`.

Use [IdleTrigger.example.toml](IdleTrigger.example.toml) as the bilingual configuration reference. Changes apply automatically within a few seconds; to reload immediately, restart IdleTrigger or run:

```powershell
.\IdleTrigger-x64.exe config:reload
```

Auto-start is stored in the current user's Windows Run registry key and is managed by the panel or CLI, not TOML.

## Logging

Enable **Debug Log** in the panel or set `logging_enabled = true`. The log is written next to the EXE, with `%TEMP%` as a fallback. It rotates at 5 MiB, retains one previous file, and includes a session identifier on each line.

## Build and Development

See [development guide](docs/development.md) for prerequisites, dual-architecture builds, resource generation, and verification commands.

## Acknowledgments

- [getlantern/systray](https://github.com/getlantern/systray): Windows tray implementation adapted from v1.2.2 (Apache-2.0)
- [BurntSushi/toml](https://github.com/BurntSushi/toml): TOML parser
- [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys): Windows API bindings
- [NoSleep](https://github.com/CHerSun/NoSleep): inspiration for the Stay Awake feature

## License

MIT
