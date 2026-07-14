//go:build devtools

package main

import (
	"fmt"
	"os"

	"github.com/JeffioZ/idletrigger/internal/devtools"
	"github.com/JeffioZ/idletrigger/internal/gdiplus"
	"github.com/JeffioZ/idletrigger/internal/inputdiag"
	"github.com/JeffioZ/idletrigger/internal/screenshot"
)

func startInputDiagnostics(config devtools.Config) func() {
	return inputdiag.Start(config.InputTrace)
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
	start() // failure preserves the popup's GDI fallback paths.
	enableConsole()
	err := run(args)
	shutdown()
	if err != nil {
		fmt.Fprintln(os.Stderr, "IdleTrigger screenshot failed:", err)
		return 1
	}
	return 0
}
