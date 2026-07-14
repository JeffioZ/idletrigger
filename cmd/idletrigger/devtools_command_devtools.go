//go:build devtools

package main

import (
	"fmt"
	"os"

	"github.com/JeffioZ/idletrigger/internal/devtools"
	"github.com/JeffioZ/idletrigger/internal/devtools/inputtrace"
	"github.com/JeffioZ/idletrigger/internal/devtools/screenshot"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/gdiplus"
)

func startInputDiagnostics(config devtools.Config) func() {
	return inputtrace.Start(config.InputTrace)
}

func runDevtoolsCommand(args []string) (int, bool) {
	if !screenshot.IsCommand(args) {
		return 0, false
	}
	return runScreenshot(args), true
}

func runScreenshot(args []string) int {
	return runScreenshotWith(args, gdiplus.Start, gdiplus.Shutdown, screenshot.Run, enableConsoleOutput)
}

func runScreenshotWith(args []string, start func() bool, shutdown func(), run func([]string) error, enableConsole func()) int {
	start() // failure preserves the control panel's GDI fallback paths.
	enableConsole()
	err := run(args)
	shutdown()
	if err != nil {
		fmt.Fprintln(os.Stderr, "IdleTrigger screenshot failed:", err)
		return 1
	}
	return 0
}
