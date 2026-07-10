# Build Instructions

> [中文版](BUILD.zh-CN.md)

## Prerequisites

- Go 1.25 or newer
- Git
- Python 3 only when regenerating icons
- `github.com/akavel/rsrc` module only when regenerating Windows resources

```powershell
go version
go mod download
```

IdleTrigger targets Windows 10 / Server 2016 or newer. Both `windows/386`
and `windows/amd64` are supported.

Windows 7 is not supported by the current build: Go 1.20 was the final Go
release that ran on Windows 7. If real demand appears, maintain a separate
legacy build based on Go 1.20 with compatible dependency versions and test it
on Windows 7; do not lower the main build's toolchain.

## Build

The repository contains architecture-specific `.syso` resources for 386 and
amd64, so either build includes the application icon, manifest, and Windows
version information. Regenerate resources when the release version changes so
Explorer's file properties match the in-app version.

```powershell
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64" # or "386"
$version = "dev"
$ldflags = "-s -w -H windowsgui -X github.com/JeffioZ/idletrigger/internal/version.Value=$version"
$output = if ($env:GOARCH -eq "amd64") { "IdleTrigger-x64.exe" } else { "IdleTrigger-x86.exe" }
go build -trimpath -ldflags=$ldflags -o $output .
```

`CGO_ENABLED=0` produces a self-contained executable that only depends on
Windows system DLLs. `-H windowsgui` prevents a console window from flashing
when the tray application starts.

## Verify

```powershell
go test ./...
go vet ./...
gofmt -l .
go mod verify
```

The release workflow performs test/vet first, builds both architectures, and
publishes `SHA256SUMS.txt` with the executables.

## Offline Build

Run `go mod vendor` while dependencies are available, then copy the repository
including `vendor/` to the offline machine:

```powershell
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64"
go build -mod=vendor -trimpath -ldflags="-s -w -H windowsgui" -o IdleTrigger-x64.exe .
```

## Regenerate Icons and Resources

The icon generator uses only the Python standard library. It stores each ICO
size as a PNG-compressed frame, supported by the Windows 10+ target platform:

```powershell
python scripts/gen_icon.py assets
```

Regenerate both architectures. Use the same version string for this command and
for `-X github.com/JeffioZ/idletrigger/internal/version.Value=...` during the
release build:

```powershell
$version = "1.3.0"
go run ./scripts/gen_resource.go -version $version
```

Commit `app.ico`, the three tray ICO files, `assets/manifest.xml`, the resource
generator, and both `.syso` files together so checked-in resources remain
reproducible.

## Development Loop

```powershell
go test ./...
go build -o IdleTrigger-dev.exe .
./IdleTrigger-dev.exe

# In a second terminal while the tray is running:
./IdleTrigger-dev.exe nosleep on
./IdleTrigger-dev.exe nosleep status
./IdleTrigger-dev.exe monitor on
```

UPX compression and code signing are optional release steps. Do not compress
debug builds because it makes diagnostics and antivirus analysis harder.
