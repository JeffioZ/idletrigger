# 🧭 IdleTrigger User Guide

[Documentation](README.md) · [简体中文](user-guide.zh-CN.md) · [Project Home](../README.md)

## 🚀 Start

IdleTrigger supports Windows 10 / Windows Server 2016 and later. Use x64 on most PCs; use x86 only on 32-bit Windows.

1. Create a writable folder you intend to keep, such as `%LOCALAPPDATA%\IdleTrigger`.
2. Download [IdleTrigger-x64.exe](https://github.com/JeffioZ/idletrigger/releases/latest/download/IdleTrigger-x64.exe) or [IdleTrigger-x86.exe](https://github.com/JeffioZ/idletrigger/releases/latest/download/IdleTrigger-x86.exe) into it.
3. Run the EXE. IdleTrigger appears in the notification area without opening a main window.
4. Left-click the tray icon for the control panel; right-click for **Open** and **Exit**.

## 🪟 Control Panel

| Section | What it controls |
| --- | --- |
| **Power Management** | Stay Awake, Idle Monitoring, idle timeout, and idle action |
| **Automatic Tasks** | Master switch, enabled count, next run, and task manager |
| **Day / Night** | Scheduled theme switching and battery/fullscreen behavior |
| **General Settings** | Global hotkeys, auto-start, debug logging, language, and configuration |
| **System Controls** | Lock, sleep, hibernate, shut down, or restart immediately |

Use `Tab` / `Shift+Tab` to move and `Space` to activate the focused control.

> [!CAUTION]
> Save your work before using **System Controls**. These actions run immediately.

## ⚡ Power Management

### Stay Awake

Stay Awake prevents automatic sleep through Windows power requests. You can optionally keep the display on. It does not simulate keyboard or mouse input.

### Idle Monitoring

Idle Monitoring uses Windows' last-input time to lock, sleep, hibernate, or shut down after real keyboard and mouse inactivity. It defaults to 30 minutes and Sleep.

The optional pre-action reminder is cancellable by input or by closing it. **Enhanced Monitoring** is available for devices or apps that repeatedly reset Windows idle time.

Stay Awake and Idle Monitoring are mutually exclusive while running. Automatic tasks can temporarily change their effective state without rewriting your manual settings.

## 🔁 Automatic Tasks

| Trigger | Available result |
| --- | --- |
| A process is running or a time window is active | Temporarily enable or pause Stay Awake or Idle Monitoring |
| Once, daily, weekly, process start, or all selected processes exit | Lock, sleep, hibernate, shut down, or restart |

Process targets can match an executable name or one exact EXE path. System actions always use a cancellable countdown of at least 10 seconds.

Tasks run only while IdleTrigger is running. Process state is checked about every five seconds, and schedules missed during sleep are not replayed after resume. Tasks use built-in actions only; they do not run custom commands.

## 🌗 Day / Night Themes

Switch Windows light and dark themes at fixed times or calculated sunrise and sunset. Optional behavior includes dark mode on battery and postponing a scheduled change during fullscreen apps or games.

Sunrise and sunset can use manual coordinates or optional IP-based location. If Windows theme settings are unavailable or blocked by policy, IdleTrigger disables this section and keeps the saved configuration intact.

## ⚙️ Configuration

| File | Purpose |
| --- | --- |
| `IdleTrigger.toml` | Settings and automatic-task rules |
| `IdleTrigger.state.json` | Scheduler state used to avoid repeated task execution |
| `IdleTrigger.log` | Optional diagnostic log |

IdleTrigger creates these files beside the EXE as needed. See [IdleTrigger.example.toml](../IdleTrigger.example.toml) for every configuration field. Valid edits apply automatically within a few seconds; reload immediately with:

```powershell
.\IdleTrigger-x64.exe config:reload
```

Auto-start is managed by the panel or CLI and is stored in the current user's Windows Run registry key.

## ⌨️ Command Line

Run the EXE without arguments to start the tray app.

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

Commands that change `nosleep` or `monitor`, plus `config:reload`, require the tray app to be running. Status queries and one-shot power actions also work without it.

## 🔄 Update or Move

Exit IdleTrigger, replace or move the EXE, and keep `IdleTrigger.toml` plus `IdleTrigger.state.json` beside it. After moving the app, launch it once from the new location before relying on auto-start.

Use [SHA256SUMS.txt](https://github.com/JeffioZ/idletrigger/releases/latest/download/SHA256SUMS.txt) to verify downloaded executables.

## 🧰 Logs

Enable **Debug Log** in the panel or set `logging_enabled = true`. Logs are written beside the EXE, with `%TEMP%` as a fallback when that folder is not writable.
