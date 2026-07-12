# Build IdleTrigger

[简体中文](BUILD.zh-CN.md)

## Requirements

- Go 1.26 or later
- Git
- `github.com/akavel/rsrc` only when regenerating Windows resources

IdleTrigger targets Windows 10 / Windows Server 2016 and later. The repository produces both `windows/amd64` and `windows/386` binaries.

Windows 7 is not supported by the main build. Go 1.20 was the last Go release to run there; a future compatibility build should be a separately maintained Go 1.20 branch with matching dependency versions and real-device validation.

```powershell
go version
go mod download
```

## Build

The architecture-specific `.syso` files contain the application icon, manifest, and Windows version metadata. They are generated build artifacts and are not committed. Regenerate them before building so Explorer properties and the app version agree.

```powershell
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64" # "386" for 32-bit Windows
$version = "dev"
go run ./scripts/gen_resource.go -version $version
$ldflags = "-s -w -H windowsgui -X github.com/JeffioZ/idletrigger/internal/version.Value=$version"
$output = if ($env:GOARCH -eq "amd64") { "IdleTrigger-x64.exe" } else { "IdleTrigger-x86.exe" }
go build -trimpath "-ldflags=$ldflags" -o $output .
```

`CGO_ENABLED=0` keeps the binary self-contained. `-H windowsgui` prevents a console window when launching the tray app.

## Verify

Run the standard local checks in Windows PowerShell 5.1 or later:

```powershell
.\scripts\check.ps1
```

`golangci-lint` is run when installed; otherwise the script prints `SKIPPED`.
Run the optional vulnerability scan while online:

```powershell
.\scripts\check.ps1 -Vulncheck
```

Without `-Vulncheck`, the vulnerability scan is not run and the script reports
that explicitly. If the system caches are not writable, set optional user-local
cache paths for the current shell before running the script:

```powershell
$env:GOCACHE = Join-Path $env:LOCALAPPDATA "IdleTrigger\cache\go-build"
$env:GOLANGCI_LINT_CACHE = Join-Path $env:LOCALAPPDATA "IdleTrigger\cache\golangci-lint"
```

Build both targets explicitly:

```powershell
$env:CGO_ENABLED = "0"
$version = "dev"
go run ./scripts/gen_resource.go -version $version
$ldflags = "-s -w -H windowsgui -X github.com/JeffioZ/idletrigger/internal/version.Value=$version"

$env:GOARCH = "amd64"
go build -trimpath "-ldflags=$ldflags" -o dist/IdleTrigger-x64.exe .

$env:GOARCH = "386"
go build -trimpath "-ldflags=$ldflags" -o dist/IdleTrigger-x86.exe .
```

The release workflow runs formatting, module, test, and vet checks, produces both executables, and publishes `SHA256SUMS.txt`.

## Regenerate Resources

The application icon and the two taskbar icons have separate artwork. Regenerate
the application ICO first, then the purpose-built taskbar variants:

```powershell
go run ./scripts/gen_app_icon/main.go assets
go run ./scripts/gen_tray_theme_icons assets
```

Then regenerate both architecture resources. Use the identical version value in the resource command and release build:

```powershell
$version = "1.3.0"
go run ./scripts/gen_resource.go -version $version
```

Commit `app.ico`, both tray ICO files, `assets/manifest.xml`, and the generators together. Do not commit `.syso` files; the release workflow regenerates them from the tag version.

## Offline Build

Vendor dependencies while online, then copy the repository including `vendor/` to the offline machine:

```powershell
go mod vendor

$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64"
go run ./scripts/gen_resource.go -version dev
go build -mod=vendor -trimpath -ldflags="-s -w -H windowsgui" -o dist/IdleTrigger-x64.exe .
```

## Development Loop

```powershell
go test ./...
go run ./scripts/gen_resource.go -version dev
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64"
go build -trimpath -ldflags="-H windowsgui" -o dist/IdleTrigger-x64-dev.exe .
.\dist\IdleTrigger-x64-dev.exe

# In a second terminal after the tray app starts:
cmd /c .\dist\IdleTrigger-x64-dev.exe nosleep on
cmd /c .\dist\IdleTrigger-x64-dev.exe nosleep status
cmd /c .\dist\IdleTrigger-x64-dev.exe monitor on
```

The dev build uses the Windows GUI subsystem so tray startup does not flash a
console window. For CLI output checks, run commands through `cmd /c` or redirect
stdout/stderr with `Start-Process`; direct PowerShell invocation of GUI-subsystem
EXEs can return before output is attached.

Code signing is an optional release step. Do not pack debug builds with UPX: it complicates diagnostics and can increase antivirus scrutiny.
