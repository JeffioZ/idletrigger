@echo off
setlocal EnableExtensions DisableDelayedExpansion
rem Usage: Start-IdleTrigger-Devtools-Warning-Preview.bat [devtools.exe]
rem Tail-call the shared launcher so dragged paths are expanded only once.
"%~dp0..\tools\devtools\Start-IdleTrigger-Devtools.bat" warning-preview "%~1"
