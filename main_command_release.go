//go:build !devtools

package main

import "github.com/JeffioZ/idletrigger/internal/devtools"

// runDevtoolsCommand leaves maintenance-only commands out of release builds.
func runDevtoolsCommand([]string) (int, bool) { return 0, false }

func startInputDiagnostics(devtools.Config) func() { return nil }
