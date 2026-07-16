@echo off
setlocal EnableExtensions DisableDelayedExpansion
rem Usage: Start-IdleTrigger-Devtools-Input-Trace.bat [devtools.exe]
rem Tail-call the shared launcher so dragged paths are expanded only once.
"%~dp0Start-IdleTrigger-Devtools.bat" input-trace "%~1"
