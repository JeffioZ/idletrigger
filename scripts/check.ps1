[CmdletBinding()]
param(
    [switch]$Vulncheck
)

$ErrorActionPreference = "Stop"

function Invoke-NativeCheck {
    param(
        [string]$Name,
        [scriptblock]$Command
    )

    Write-Host "==> $Name"
    & $Command
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        Write-Host "FAILED: $Name failed with exit code $exitCode."
        exit $exitCode
    }
}

Invoke-NativeCheck "go mod verify" { go mod verify }

Write-Host "==> gofmt -l ."
$unformatted = @(gofmt -l .)
$gofmtExitCode = $LASTEXITCODE
if ($gofmtExitCode -ne 0) {
    Write-Host "FAILED: gofmt failed with exit code $gofmtExitCode."
    exit $gofmtExitCode
}
if ($unformatted.Count -ne 0) {
    Write-Host "FAILED: unformatted files:"
    $unformatted | ForEach-Object { Write-Host $_ }
    exit 1
}

Invoke-NativeCheck "git diff --check" { git diff --check }
Invoke-NativeCheck "go test -count=1 ./..." { go test -count=1 ./... }
Invoke-NativeCheck "go vet ./..." { go vet ./... }

$golangci = Get-Command golangci-lint -ErrorAction SilentlyContinue
$golangciSkipped = $null -eq $golangci
if ($golangciSkipped) {
    Write-Host "SKIPPED: golangci-lint is not installed."
} else {
    Invoke-NativeCheck "golangci-lint" { & $golangci.Source run --timeout=5m --disable=errcheck ./... }
}

if ($Vulncheck) {
    Invoke-NativeCheck "govulncheck v1.1.4" { go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./... }
} else {
    Write-Host "NOT RUN: govulncheck (pass -Vulncheck to run it)."
}

if (-not $golangciSkipped -and $Vulncheck) {
    Write-Host "All requested checks passed."
} else {
    Write-Host "Core checks passed; see skipped or not-run checks above."
}
