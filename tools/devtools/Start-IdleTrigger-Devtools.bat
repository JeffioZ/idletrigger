@echo off
setlocal EnableExtensions DisableDelayedExpansion

rem Shared PowerShell 7 bridge used by the mode-specific wrappers.
set "MODE=%~1"
if not defined MODE (
    echo ERROR: This helper must be called by a Start-IdleTrigger-Devtools-*.bat wrapper.
    exit /b 2
)

set "PWSH=%ProgramFiles%\PowerShell\7\pwsh.exe"
if not exist "%PWSH%" (
    set "PWSH="
    for %%I in (pwsh.exe) do set "PWSH=%%~$PATH:I"
)
if not defined PWSH (
    echo ERROR: PowerShell 7 or later is required.
    echo Install PowerShell 7, then rerun the wrapper.
    exit /b 2
)

"%PWSH%" -NoLogo -NoProfile -File "%~dp0Invoke-IdleTrigger-Devtools.ps1" -Mode "%MODE%" -ExePath "%~2" -IdleSeconds "%~3"
exit /b %ERRORLEVEL%
