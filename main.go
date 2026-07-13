package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/cli"
	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/darkmode"
	"github.com/JeffioZ/idletrigger/internal/devtools"
	"github.com/JeffioZ/idletrigger/internal/dpi"
	"github.com/JeffioZ/idletrigger/internal/i18n"
	"github.com/JeffioZ/idletrigger/internal/inputdiag"
	"github.com/JeffioZ/idletrigger/internal/ipc"
	mylog "github.com/JeffioZ/idletrigger/internal/log"
	"github.com/JeffioZ/idletrigger/internal/screenshot"
	"github.com/JeffioZ/idletrigger/internal/singleinstance"
	"github.com/JeffioZ/idletrigger/internal/tray"
	"github.com/JeffioZ/idletrigger/internal/version"
)

func main() {
	if screenshot.IsCommand(os.Args[1:]) {
		enableConsoleOutput()
		if err := screenshot.Run(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "IdleTrigger screenshot failed:", err)
			os.Exit(1)
		}
		return
	}

	dpi.Enable()
	darkmode.Enable()
	developerTools := devtools.Load()

	isCLI := false
	startMinimized := false
	startupDelay := 0
	for _, a := range os.Args[1:] {
		if a == "--minimized" {
			isCLI = false
			startMinimized = true
		} else if strings.HasPrefix(a, "--delay=") {
			v, err := strconv.Atoi(strings.TrimPrefix(a, "--delay="))
			if err == nil && v > 0 && v <= 60 {
				startupDelay = v
			}
			continue
		} else {
			isCLI = true
		}
	}
	if !isCLI {
		guard, primary, err := singleinstance.Acquire()
		if err != nil {
			fmt.Fprintln(os.Stderr, "IdleTrigger startup failed:", err)
			return
		}
		if !primary {
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if _, ok := ipc.Send("open"); ok {
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			return
		}
		defer guard.Release()
	}

	cfg, err := config.Load()
	configLoadErr := err
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("auto", "warning_config_defaults"), err)
		cfg = config.DefaultConfig()
	}

	if isCLI {
		enableConsoleOutput()
		cli.Run(cfg.Language)
		return
	}

	// GUI mode
	exePath, _ := os.Executable()
	if developerTools.ForceLog {
		cfg.LoggingEnabled = true
	}
	mylog.Init(cfg.LoggingEnabled, filepath.Dir(exePath))
	defer mylog.Close()
	mylog.Info("IdleTrigger starting: version=%s mode=GUI", version.Value)
	if developerTools.Enabled {
		mylog.Info("Developer tools active: modes=%s idle_monitor_seconds=%d config_overrides=runtime_only", developerTools.Summary(), developerTools.IdleMonitorSeconds)
	}
	for _, notice := range developerTools.Notices {
		mylog.Info("%s", notice)
	}
	if stopInputDiagnostics := inputdiag.Start(developerTools.InputTrace); stopInputDiagnostics != nil {
		defer stopInputDiagnostics()
	}
	if configLoadErr != nil {
		mylog.Info("Config load failed; using defaults without modifying the file: %v", configLoadErr)
	}

	if startupDelay > 0 {
		time.Sleep(time.Duration(startupDelay) * time.Second)
	}
	tray.Run(cfg, tray.Callbacks{ShowPopupOnStart: !startMinimized, DeveloperTools: developerTools})
}

func enableConsoleOutput() {
	if bindStandardConsoleFiles() {
		return
	}

	kernel32 := windows.NewLazySystemDLL("kernel32.dll")

	attach := kernel32.NewProc("AttachConsole")
	const ATTACH_PARENT_PROCESS = 0xFFFFFFFF
	ret, _, _ := attach.Call(uintptr(ATTACH_PARENT_PROCESS))
	if ret == 0 {
		alloc := kernel32.NewProc("AllocConsole")
		alloc.Call()
	}

	bindStandardConsoleFiles()
}

func bindStandardConsoleFiles() bool {
	hOut, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	hErr, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	bound := false
	if usableStdHandle(hOut) {
		os.Stdout = os.NewFile(uintptr(hOut), "/dev/stdout")
		bound = true
	}
	if usableStdHandle(hErr) {
		os.Stderr = os.NewFile(uintptr(hErr), "/dev/stderr")
		bound = true
	}
	return bound
}

func usableStdHandle(h windows.Handle) bool {
	if h == 0 || h == windows.InvalidHandle {
		return false
	}
	fileType, err := windows.GetFileType(h)
	return err == nil && fileType != windows.FILE_TYPE_UNKNOWN
}
