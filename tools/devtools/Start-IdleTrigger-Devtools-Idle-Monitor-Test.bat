@echo off
setlocal EnableExtensions DisableDelayedExpansion
rem Usage: Start-IdleTrigger-Devtools-Idle-Monitor-Test.bat [devtools.exe] [10..600]
rem Tail-call the shared launcher so dragged paths are expanded only once.
"%~dp0Start-IdleTrigger-Devtools.bat" idle-monitor-test "%~1" "%~2"
