# IdleTrigger

> 📖 [中文版](README.zh-CN.md)

**System idle monitor, power scheduler & sleep preventer for Windows**

A lightweight, single-EXE utility that lives in your system tray.  It can:

- **Prevent sleep** — keep the system awake by resetting the Windows idle timer
- **Auto-trigger** — sleep / hibernate / shutdown / lock after idle timeout
- **One-shot CLI** — trigger actions from terminal, scripts, or AI agents
- **IPC** — CLI commands seamlessly communicate with the running tray instance

## Features

- **System tray** — right-click menu with full settings control, no window
- **Stay Awake** — prevent Windows from sleeping (also keeps screen on)
- **Idle monitor** — auto-trigger after N minutes of no keyboard/mouse input
  (5 / 10 / 30 / 60 / 120 min, configurable from tray submenu)
- **Global hotkeys** — `Win+Shift+S` sleep / `+L` lock / `+N` toggle stay-awake
- **Battery awareness** — auto-disable stay-awake when on battery
- **Process watching** — auto-enable stay-awake when specific apps are running
- **IPC named pipe** — CLI commands talk to the running tray instance
- **Capability detection** — unavailable actions (e.g. hibernate) are auto-disabled
- **Single EXE** — fully statically linked; copy anywhere, no install
- **Plain-text config** — `IdleTrigger.toml` next to the EXE, edit with Notepad
- **Multi-language** — English / 中文. Auto-detects OS language (Chinese on zh-CN Windows, English otherwise), manually switchable from tray menu
- **DPI & dark mode** — Per-Monitor V2, native dark context menus and dialogs
- **Platform**: Windows 7+, 32-bit and 64-bit. The 32-bit EXE runs on both architectures

## Quick Start

1. Download `IdleTrigger.exe` from [Releases](https://github.com/jeffio/idletrigger/releases)
2. Double-click → tray icon appears (slate blue power symbol)
3. Right-click to configure; or edit `IdleTrigger.toml`

## CLI Usage

```
IdleTrigger sleep                 Suspend system
IdleTrigger hibernate             Hibernate system
IdleTrigger shutdown              Shut down system
IdleTrigger lock                  Lock workstation

IdleTrigger nosleep on            Keep system awake
IdleTrigger nosleep on --screen   Keep awake + screen on
IdleTrigger nosleep off           Restore normal power management
IdleTrigger nosleep toggle        Toggle stay-awake
IdleTrigger nosleep status        Show stay-awake state

IdleTrigger monitor on            Enable idle monitor (IPC → tray)
IdleTrigger monitor off           Disable idle monitor (IPC → tray)

IdleTrigger autostart enable      Enable auto-start on login
IdleTrigger autostart disable     Disable auto-start
IdleTrigger autostart status      Show auto-start state

IdleTrigger status                Show full system state
IdleTrigger version               Print version
```

Run without arguments to launch the system-tray GUI.

**AI agent / script integration**: when the tray is running, `nosleep` and
`monitor` commands are forwarded via a named pipe (`\\.\pipe\IdleTrigger`).
One-shot actions (`sleep`, `lock`, …) execute directly.

## Configuration (`IdleTrigger.toml`)

```toml
# IdleTrigger Configuration
# Edit directly; tray picks up changes on restart (or via CLI "config:reload").

language = "auto"                  # "auto" (follow OS), "en", "zh-CN"
idle_timeout_minutes = 30          # 0 = disable idle monitor
idle_action = "sleep"              # sleep | hibernate | shutdown | lock
idle_warning_seconds = 30          # pre-trigger notification; 0 = off

nosleep_enabled = false            # stay awake
keep_screen_on = false             # also keep display on
nosleep_on_battery = false         # allow stay-awake on battery power
nosleep_battery_threshold = 20     # min battery % for stay-awake

hotkeys_enabled = false            # Win+Shift+S/L/N

process_watch_enabled = false      # auto stay-awake when apps run
process_watch_list = []            # e.g. ["chrome.exe", "powerpnt.exe"]

start_minimized = true
logging_enabled = false            # debug log to IdleTrigger.log
autostart_enabled = false
```

## Project Structure

```
IdleTrigger/
├── main.go                          # Entry point: CLI vs GUI dispatch
├── assets/
│   ├── app.ico                      # EXE icon (16/32/48/256)
│   ├── icon.go                      # go:embed bridges
│   ├── icon_default.ico             # Tray: idle state (slate blue)
│   ├── icon_monitor.ico             # Tray: monitor active (amber)
│   ├── icon_active.ico              # Tray: stay-awake active (green)
│   ├── manifest.xml                 # DPI & dark mode manifest
├── scripts/
│   └── gen_icon.py                  # Icon generator (dev-only)
├── internal/
│   ├── actions/actions.go           # Win32: Sleep, Hibernate, Shutdown, Lock
│   ├── autostart/autostart.go       # Registry Run-key management
│   ├── cli/cli.go                   # CLI dispatch + IPC client
│   ├── config/config.go             # TOML config load/save
│   ├── darkmode/darkmode.go         # uxtheme ordinal 135/136
│   ├── dialog/dialog.go             # TaskDialog (dark-mode aware)
│   ├── dpi/dpi.go                   # Per-Monitor V2
│   ├── hotkey/hotkey.go             # Global hotkeys (Win+Shift+S/L/N)
│   ├── i18n/                        # Multi-language (en, zh-CN)
│   │   ├── i18n.go
│   │   └── locales/{en,zh-CN}.json
│   ├── ipc/ipc.go                   # Named-pipe IPC: Server + Client
│   ├── monitor/monitor.go           # GetLastInputInfo idle detection
│   ├── nosleep/nosleep.go           # SetThreadExecutionState
│   ├── notify/notify.go             # Balloon-tip notifications
│   ├── power/power.go               # Battery status + sleep capabilities
│   ├── processwatcher/processwatcher.go  # Process-list watcher
│   └── tray/tray.go                 # System-tray menu + IPC server
├── rsrc_windows_386.syso          # Compiled resource (icon + manifest)
├── go.mod  go.sum  LICENSE  .gitattributes  .gitignore
├── README.md  README.zh-CN.md  BUILD.md  BUILD.zh-CN.md
```

## Tray Menu Reference

```
Sleep / Hibernate / Shut Down / Lock
─────────────────
Stay Awake
─────────────────
Idle Monitor
  Timeout ▸  5 / 10 / 30 / 60 / 120 min
  Trigger Action ▸  Sleep / Hibernate / Shut Down / Lock
─────────────────
Global Hotkeys
Start with Windows
Language ▸  English / 简体中文
─────────────────
Config
About
─────────────────
Exit
```

## Acknowledgments

- [getlantern/systray](https://github.com/getlantern/systray) — cross-platform system tray library
- [BurntSushi/toml](https://github.com/BurntSushi/toml) — TOML parser for Go
- [golang.org/x/sys](https://golang.org/x/sys) — Windows API bindings
- [NoSleep](https://github.com/CHerSun/NoSleep) — inspiration for the sleep-prevention feature

## Development

This project was built via **Vibe Coding** — an AI-assisted
development workflow. All code, documentation, and design decisions
were created collaboratively between human direction and AI generation.

## License

MIT
