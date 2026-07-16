@echo off
setlocal EnableExtensions DisableDelayedExpansion
rem Usage: Start-IdleTrigger-Devtools-UI-Capture.bat [devtools.exe]
rem Tail-call the shared launcher so dragged paths are expanded only once.
"%~dp0Start-IdleTrigger-Devtools.bat" ui-capture "%~1"
