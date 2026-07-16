# IdleTrigger

[简体中文](README.zh-CN.md)

**A lightweight Windows tray utility for idle actions, automatic tasks, stay-awake control, and scheduled theme switching.**

IdleTrigger is a single executable with no runtime dependencies beyond Windows system DLLs. It stays out of the way in the notification area and keeps its settings in a readable TOML file beside the executable.

## What It Does

- **Idle monitor**: after a chosen period without keyboard or mouse input, lock, sleep, hibernate, or shut down the PC.
- **Pre-action reminder**: show a non-activating reminder before an idle action; mouse or keyboard input, or closing the reminder, cancels the pending action.
- **Stay Awake**: prevent automatic sleep, optionally keeping the display on.
- **Automatic tasks**: combine time windows, one-time/daily/weekly schedules, or process conditions with supported built-in actions.
- **Day/Night theme**: switch Windows themes at fixed times or from calculated sunrise and sunset; optionally use dark mode on battery and pause switching during fullscreen apps, presentations, or foreground games.
- **System controls**: lock, sleep, hibernate, shut down, or restart from the control panel or command line.
- **Running-instance control**: control the active tray instance through a per-session named pipe.

## Requirements

- Windows 10 / Windows Server 2016 or later
- x64 build for most PCs; x86 build for 32-bit Windows
- Theme switching requires Windows Personalize settings. It may be unavailable on Server Core or policy-managed desktops.

Windows 7 is intentionally not supported by the main build. See [development guide](docs/development.md) for the compatibility rationale.

## Quick Start

1. Download `IdleTrigger-x64.exe` from [Releases](https://github.com/JeffioZ/idletrigger/releases).
2. Run it. The app appears in the notification area without opening a main window.
3. Left-click the tray icon to open or close the control panel. Right-click it for the native **Open** and **Exit** menu.
4. Use the panel for common settings; edit `IdleTrigger.toml` beside the EXE for advanced settings.

The control panel follows Windows light/dark mode, responds to DPI changes, and stays open until you close it or left-click the tray icon again. Tooltips explain each available option.

## Using the Control Panel

- Blue controls are enabled or selected; neutral controls are available but not selected. **Exit** is red because it stops all IdleTrigger features.
- Hover **System Controls** or **Language Settings** to open their menus. **System Controls** run immediately; save your work before choosing Sleep, Hibernate, Shut Down, or Restart.
- Use the mouse or `Tab` / `Shift+Tab` to move between controls, then press `Space` to activate the focused control. The keyboard focus has a visible outline.
- **Power Management** groups Stay Awake, Idle Monitoring, and their related settings. Its two primary toggles show manual configuration; automatic tasks never rewrite them, and each tooltip shows both the manual setting and current runtime status.
- **Automatic Tasks** is an independent section that shows the enabled-task count and next scheduled time. Enable or pause all tasks directly from the main panel, or open **Manage Automatic Tasks** to manage rules and choose processes without editing TOML; pausing does not delete rules. While its manager is open, the control panel is temporarily unavailable and resumes when the manager closes. Use **Edit Config** for advanced settings such as location and detailed theme rules.

## Screenshots

<img src="docs/images/panel-en-light.png" alt="IdleTrigger control panel in English light mode" width="420">

<details>
<summary>More themes and languages</summary>

| English dark | Simplified Chinese light | Simplified Chinese dark |
| --- | --- | --- |
| <img src="docs/images/panel-en-dark.png" alt="IdleTrigger control panel in English dark mode" width="260"> | <img src="docs/images/panel-zh-light.png" alt="IdleTrigger control panel in Simplified Chinese light mode" width="260"> | <img src="docs/images/panel-zh-dark.png" alt="IdleTrigger control panel in Simplified Chinese dark mode" width="260"> |

</details>

## Idle Monitor

The idle monitor is enabled by default with a 30-minute idle time and Sleep as its action. Available panel idle-time choices are:

`1, 2, 3, 5, 10, 15, 30 minutes; 1, 2, 5 hours`.

The monitor uses Windows `GetLastInputInfo` to observe real keyboard and mouse activity. A newly started or re-enabled monitor begins a fresh idle window; it never acts immediately because the machine had already been idle before IdleTrigger started. After an action is triggered, the idle window is reset before monitoring continues.

The **Enable Pre-action Reminder** switch shows a non-activating prompt before the action. Any keyboard or mouse input cancels the pending action; closing the prompt does the same. Set `idle_warning_seconds = 0` for silent operation.

If a device, driver, or app refreshes Windows idle time at a fixed interval and prevents system sleep or idle actions, use the **Enable Enhanced Monitoring** switch. It is off by default; when enabled, IdleTrigger first logs and learns a stable reset pattern, then keeps a more robust idle timer. Normal keyboard or mouse input still resets idle time, and logs continue to record why each reset was accepted or ignored.

## Automatic Tasks

Open the manager from the control panel's independent **Automatic Tasks** section to create, edit, delete, enable, or disable rules. An empty list explains how to create the first task; Edit, Delete, and Enable/Disable are unavailable without a selection, and deletion requires confirmation. The editor progressively shows only the fields required by its **Basics**, **Trigger Conditions**, and **Action Options** sections. Task names have an input cue and can still be generated automatically; active days use multi-select buttons plus **Weekdays** and **Every day** shortcuts. Validation explains and focuses the first invalid field. The manager is modal to the control panel, and the process picker is modal to the task editor. Closing the editor returns to the task list and confirms before discarding changes. Supported state actions are enable or pause Stay Awake and enable or pause the idle monitor; system actions are Lock, Sleep, Hibernate, Shut Down, and Restart. State actions can run while selected processes are running or during a time window. A pause temporarily overrides the corresponding manual setting and releases it when the task condition ends. System actions can run once, daily, weekly, when any selected process starts, or after all selected processes exit; a scheduled system action can also require a process condition.

The process picker loads names first and fills descriptions in the background, stays within a bounded scrolling window, and provides a search cue plus explicit **Refresh** and **Browse** buttons. When the picker becomes active again with a stale snapshot, it performs a lightweight refresh while preserving search, sorting, checks, and the visible position where possible; manual Refresh remains available. Choice popups, the task list, the process list, and the current-selection preview share the same themed scrollbar. Its sortable Process, Description, and Instances columns contain one row per executable name; clicking the checkbox or process name selects every same-name instance. Use **Browse** to choose a specific Windows EXE instead. Exact-file choices appear only in the current-selection preview, so paths are not mixed into the running-process list. PIDs and descriptions are never stored as rule identity.

A **When any process starts** task fires only when the selected set changes from none running to at least one running. Processes already running when IdleTrigger starts do not backfill an event, and later same-name instances do not trigger duplicates. A process-exit task waits until every matching instance has exited, then applies a 5-second grace period; brief exits or restarts inside that grace period do not produce repeated actions.

Process discovery uses the Windows Toolhelp process list. Name matching does not open processes. Description enrichment opens at most one accessible instance per executable name with `PROCESS_QUERY_LIMITED_INFORMATION`; exact-path rules request the same limited access only for matching names. Protected processes remain available by name when Windows denies metadata access. Browsed files are validated and read for description only; IdleTrigger never launches them. IdleTrigger does not request debug privilege, read process memory, inject code, terminate processes, install a service, or create Windows Task Scheduler entries.

Every automatic system action displays a cancellable countdown of at least 10 seconds. If multiple system actions become due together, confirming one clears the remaining queued actions for that occurrence instead of cascading through them. Rules work only while IdleTrigger is running and can invoke built-in actions only—custom commands, scripts, and arbitrary program launches are intentionally unsupported. Manual panel settings and automatic-task requests remain independent; ending a task does not rewrite a manual toggle.

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

Commands that change `nosleep` or `monitor` state, plus `config:reload`, forward to the active tray instance through `\\.\pipe\IdleTrigger-<session>` and require the tray app to be running. Status queries still return a result when the tray app is not running. One-shot power actions execute directly.

## Configuration

IdleTrigger creates and maintains `IdleTrigger.toml` next to the EXE. It adds missing keys and refreshed comments when the bundled configuration template changes, while retaining valid existing values. It does not rewrite the file on every run. Automatic rules are stored in this TOML file; occurrence bookkeeping is kept separately in `IdleTrigger.state.json` so normal scheduler ticks never rewrite user settings.

Use [IdleTrigger.example.toml](IdleTrigger.example.toml) as the bilingual top-level configuration reference. Automatic-task tables are normally created and maintained by the task manager. Saved changes apply automatically within a few seconds. To apply a change immediately, restart IdleTrigger or run:

```powershell
.\IdleTrigger-x64.exe config:reload
```

Auto-start is stored in the current user's Windows Run registry key and is managed by the panel or CLI, not TOML.

## Logging

Enable **Debug Log** in the panel or set `logging_enabled = true`. The log is written next to the EXE, with `%TEMP%` as a fallback. It rotates at 5 MiB and retains one previous file as `IdleTrigger.log.1`.

Each line includes a startup session identifier, making separate runs easy to distinguish:

```text
[2026-07-11 12:34:56.789] [session:18a0f0-2b4c] Idle monitor started
```

## Build and Development

See [development guide](docs/development.md) for prerequisites, dual-architecture builds, resource generation, and verification commands.

## Project Layout

```text
cmd/idletrigger/            Application entry point and generated Windows resources
build/windows/              Manifest and checked-in application/tray icons
docs/                       Development guide, roadmap, and README screenshots
internal/app/               Serialized application state and feature coordination
internal/automation/        Automatic-task model, validation, and runtime state file
internal/feature/           Idle, keep-awake, automatic-rule, and theme features
internal/ui/                Control panel, task/process dialogs, warnings, and tray icon
internal/platform/windows/  Native Windows integrations, process metadata, and system actions
internal/config/            TOML load, validation, migration, and atomic save
internal/devtools/          Build-tagged diagnostics and screenshot support
tools/                      Checks, generators, and screenshot automation
```

## Acknowledgments

- [getlantern/systray](https://github.com/getlantern/systray): Windows tray implementation adapted from v1.2.2 (Apache-2.0)
- [BurntSushi/toml](https://github.com/BurntSushi/toml): TOML parser
- [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys): Windows API bindings
- [NoSleep](https://github.com/CHerSun/NoSleep): inspiration for the Stay Awake feature

## License

MIT
