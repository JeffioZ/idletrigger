# Build Instructions

> 📖 [中文版](BUILD.zh-CN.md)

## Prerequisites

- **Go 1.21+** — [download](https://go.dev/dl/)
- **Git** — for dependency resolution (or use `GOPROXY`)

Verify:

```
go version   # → go version go1.21.x windows/amd64
```

## One-command Build

```powershell
cd IdleTrigger
$env:CGO_ENABLED = "0"
go mod tidy        # download dependencies (first time only)
go build -ldflags="-s -w -H windowsgui" -o IdleTrigger.exe .
```

| Flag / Env | Effect |
|------------|--------|
| `CGO_ENABLED=0` | **Force pure-Go static linking** — no C runtime, no external DLLs |
| `-H windowsgui` | **GUI subsystem — no console flash on double-click** |
| `-s` | Strip symbol table |
| `-w` | Strip DWARF debug info |

These together produce a **single, fully self-contained EXE** (~5 MB).
The only "dependencies" are Windows built-in DLLs (kernel32, user32, …)
that ship with every Windows installation since XP.


## Compress Further (optional)

After the Go build, run [UPX](https://upx.github.io/) for maximum compression:

```powershell
upx --best --lzma IdleTrigger.exe
```

This typically shrinks the binary to **~1.5 MB**.

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/BurntSushi/toml` | TOML config parsing |
| `github.com/getlantern/systray` | Cross-platform system tray |
| `golang.org/x/sys` | Windows API bindings |

All dependencies resolve automatically on `go build` / `go mod tidy`.

Note: Windows XP support requires Go ≤ 1.20 and a suitable C toolchain.
With Go 1.21+ the minimum supported Windows version is Windows 7 (or
Windows Server 2008 R2).

## Offline / Air-gapped Builds

```powershell
go mod vendor     # vendor all dependencies
$env:CGO_ENABLED = "0"
go build -mod=vendor -ldflags="-s -w -H windowsgui" -o IdleTrigger.exe .
```

## Single EXE Guarantee

IdleTrigger is a **true single-file executable**:

- **Pure Go** — `CGO_ENABLED=0` disables CGo entirely; no MinGW, no MSVC runtime
- **Static linking** — all Go packages and embedded assets are compiled into one `.exe`
- **System DLLs only** — the binary only calls Windows built-in DLLs (kernel32.dll,
  user32.dll, powrprof.dll, advapi32.dll) which exist on every Windows since XP
- **Embedded resources** — tray icons and locales via `//go:embed`; EXE icon and manifest via `.syso` resource

✅ Copy `IdleTrigger.exe` to any folder on any 32-bit or 64-bit Windows machine and it just works.
No installer, no runtime, no extra files required.

## Generate a New Tray Icon

Edit `scripts/gen_icon.py`, then:

```powershell
python scripts/gen_icon.py assets/
```

Rebuild the EXE to embed the new icon.

## Development Loop

```powershell
# Build & run GUI
go build -o IdleTrigger.exe . && .\IdleTrigger.exe

# Build & run CLI
go build -o IdleTrigger.exe . && .\IdleTrigger.exe lock

# Test IPC (tray must be running in another terminal)
.\IdleTrigger.exe nosleep on
.\IdleTrigger.exe nosleep status
.\IdleTrigger.exe monitor on
```
