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
	"github.com/JeffioZ/idletrigger/internal/dpi"
	"github.com/JeffioZ/idletrigger/internal/i18n"
	mylog "github.com/JeffioZ/idletrigger/internal/log"
	"github.com/JeffioZ/idletrigger/internal/tray"
	"github.com/JeffioZ/idletrigger/internal/version"
)

func main() {
	dpi.Enable()
	darkmode.Enable()

	isCLI := false
	startupDelay := 0
	for _, a := range os.Args[1:] {
		if a == "--minimized" {
			isCLI = false
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

	cfg, err := config.Load()
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
	mylog.Init(cfg.LoggingEnabled, filepath.Dir(exePath))
	defer mylog.Close()
	mylog.Info("IdleTrigger starting: version=%s mode=GUI", version.Value)

	if startupDelay > 0 {
		time.Sleep(time.Duration(startupDelay) * time.Second)
	}
	tray.Run(cfg, tray.Callbacks{})
}

func enableConsoleOutput() {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")

	attach := kernel32.NewProc("AttachConsole")
	const ATTACH_PARENT_PROCESS = 0xFFFFFFFF
	ret, _, _ := attach.Call(uintptr(ATTACH_PARENT_PROCESS))
	if ret == 0 {
		alloc := kernel32.NewProc("AllocConsole")
		alloc.Call()
	}

	hOut, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	hErr, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if hOut != windows.InvalidHandle {
		os.Stdout = os.NewFile(uintptr(hOut), "/dev/stdout")
	}
	if hErr != windows.InvalidHandle {
		os.Stderr = os.NewFile(uintptr(hErr), "/dev/stderr")
	}
}
