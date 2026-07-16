#requires -Version 7.0

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateSet('idle-monitor-test', 'input-trace', 'ui-capture', 'warning-preview')]
    [string]$Mode,

    [AllowEmptyString()]
    [string]$ExePath = '',

    [AllowEmptyString()]
    [string]$IdleSeconds = ''
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Set-StrictMode -Version Latest

function Stop-WithError([string]$Message, [int]$ExitCode = 2) {
    Write-Host "ERROR: $Message"
    exit $ExitCode
}

$parsedIdleSeconds = 10
if ($Mode -eq 'idle-monitor-test' -and -not [string]::IsNullOrWhiteSpace($IdleSeconds)) {
    $parsedIdleSeconds = 0
    $valid = [int]::TryParse(
        $IdleSeconds,
        [Globalization.NumberStyles]::None,
        [Globalization.CultureInfo]::InvariantCulture,
        [ref]$parsedIdleSeconds
    )
    if (-not $valid) {
        Stop-WithError 'Idle-monitor test seconds must be an integer from 10 to 600.'
    }
}
if ($Mode -eq 'idle-monitor-test' -and ($parsedIdleSeconds -lt 10 -or $parsedIdleSeconds -gt 600)) {
    Stop-WithError 'Idle-monitor test seconds must be from 10 to 600.'
}

if ([string]::IsNullOrWhiteSpace($ExePath)) {
    $repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
    $ExePath = Join-Path $repoRoot 'dist\IdleTrigger-x64-devtools.exe'
}
try {
    $resolvedExe = [IO.Path]::GetFullPath($ExePath)
} catch {
    Stop-WithError "Invalid EXE path: $ExePath"
}
if ([IO.Path]::GetExtension($resolvedExe) -ine '.exe') {
    Stop-WithError "Target must be an .exe file: $resolvedExe"
}
if (-not (Test-Path -LiteralPath $resolvedExe -PathType Leaf)) {
    Stop-WithError "Developer-tools EXE was not found: $resolvedExe`nBuild it with -tags devtools or drag a *-devtools.exe onto the wrapper."
}

$stdoutFile = New-TemporaryFile
$stderrFile = New-TemporaryFile
try {
    $versionProcess = Start-Process -FilePath $resolvedExe -ArgumentList 'version' -WindowStyle Hidden -Wait -PassThru `
        -RedirectStandardOutput $stdoutFile.FullName -RedirectStandardError $stderrFile.FullName
    $versionOutput = @(
        Get-Content -Raw -Encoding UTF8 -LiteralPath $stdoutFile.FullName
        Get-Content -Raw -Encoding UTF8 -LiteralPath $stderrFile.FullName
    ) -join [Environment]::NewLine
    $versionExitCode = $versionProcess.ExitCode
} finally {
    Remove-Item -LiteralPath $stdoutFile.FullName, $stderrFile.FullName -Force -ErrorAction SilentlyContinue
}
if ($versionExitCode -ne 0) {
    Stop-WithError "Target version check failed with exit code $versionExitCode`: $resolvedExe"
}
$versionOutput = $versionOutput.Trim()
if ($versionOutput -notmatch '(?i)devtools') {
    Stop-WithError "Target is not a devtools build: $resolvedExe`nVersion output: $versionOutput"
}

$running = @(Get-Process -Name 'IdleTrigger*' -ErrorAction SilentlyContinue)
if ($running.Count -gt 0) {
    $processList = ($running | ForEach-Object { "$($_.ProcessName)[$($_.Id)]" }) -join ', '
    Stop-WithError "IdleTrigger is already running: $processList`nExit it first; developer variables only apply to a new process." 3
}

$derivedVariables = @(
    'IDLETRIGGER_DEVTOOLS_LOG',
    'IDLETRIGGER_DEVTOOLS_IDLE_MONITOR_SECONDS',
    'IDLETRIGGER_DEVTOOLS_INPUT_TRACE',
    'IDLETRIGGER_DEVTOOLS_CAPTURE_PANEL',
    'IDLETRIGGER_DEVTOOLS_WARNING_PREVIEW'
)
foreach ($name in $derivedVariables) {
    [Environment]::SetEnvironmentVariable($name, $null, 'Process')
}
$env:IDLETRIGGER_DEVTOOLS = '1'

switch ($Mode) {
    'idle-monitor-test' {
        $env:IDLETRIGGER_DEVTOOLS_IDLE_MONITOR_SECONDS = $parsedIdleSeconds.ToString([Globalization.CultureInfo]::InvariantCulture)
    }
    'input-trace' { $env:IDLETRIGGER_DEVTOOLS_INPUT_TRACE = '1' }
    'ui-capture' { $env:IDLETRIGGER_DEVTOOLS_CAPTURE_PANEL = '1' }
    'warning-preview' { $env:IDLETRIGGER_DEVTOOLS_WARNING_PREVIEW = '1' }
}

Write-Host ''
Write-Host '=== IdleTrigger developer-tools launch ==='
Write-Host "Mode: $Mode"
Write-Host "Target EXE: $resolvedExe"
Write-Host "Version: $versionOutput"
Write-Host "PowerShell: $($PSVersionTable.PSVersion)"
Write-Host 'IDLETRIGGER_DEVTOOLS=1'
foreach ($name in $derivedVariables) {
    $value = [Environment]::GetEnvironmentVariable($name, 'Process')
    if ($null -ne $value) { Write-Host "$name=$value" }
}
if ($Mode -eq 'idle-monitor-test') {
    Write-Host 'Safety: monitor is forced on; action is Lock; warning is 5 seconds.'
    Write-Host 'Safety: Stay Awake mutual exclusion still applies; configuration is not written back.'
}
if ($Mode -eq 'input-trace') {
    Write-Host 'Privacy: input trace records keyboard key codes, injection flags, and mouse metadata to the debug log.'
}
Write-Host 'The script waits for the single-instance GUI process to exit.'
Write-Host ''

$process = Start-Process -FilePath $resolvedExe -Wait -PassThru
Write-Host ''
Write-Host "IdleTrigger developer-tools process exited with code $($process.ExitCode)."
exit $process.ExitCode
