//go:build devtools

package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/JeffioZ/idletrigger/internal/i18n"
	"github.com/JeffioZ/idletrigger/internal/monitor"
)

func runDeveloperCommand(lang, command string) bool {
	if command != "diagnostics" {
		return false
	}
	cmdDiagnostics(lang)
	return true
}

func developerUsage(lang string) string {
	if i18n.ResolveLanguage(lang) == "zh-CN" {
		return "\n开发命令：\n  diagnostics  查看原始空闲计时（idle [--watch]）"
	}
	return "\nDeveloper commands:\n  diagnostics  Raw idle timing (idle [--watch])"
}

func cmdDiagnostics(lang string) {
	if len(os.Args) < 3 || os.Args[2] != "idle" {
		fmt.Fprintln(os.Stderr, diagnosticsUsage(lang))
		os.Exit(1)
	}
	watch := false
	for _, arg := range os.Args[3:] {
		if arg == "--watch" || arg == "-w" {
			watch = true
		}
	}
	for {
		snap, err := monitor.Snapshot()
		if err != nil {
			printError(lang, fmt.Sprintf(diagnosticsErrorFormat(lang), err))
			os.Exit(1)
		}
		fmt.Printf("idle_diagnostics tick_now=%d tick32=%d last_input=%d raw_delta_ms=%d idle=%s\n",
			snap.NowTick64, snap.NowTick32, snap.LastInputTick, snap.RawDeltaMS, snap.Idle.Round(time.Millisecond))
		if !watch {
			return
		}
		time.Sleep(time.Second)
	}
}

func diagnosticsUsage(lang string) string {
	if i18n.ResolveLanguage(lang) == "zh-CN" {
		return "用法：IdleTrigger diagnostics idle [--watch]"
	}
	return "Usage: IdleTrigger diagnostics idle [--watch]"
}

func diagnosticsErrorFormat(lang string) string {
	if i18n.ResolveLanguage(lang) == "zh-CN" {
		return "空闲诊断失败：%v"
	}
	return "Idle diagnostics failed: %v"
}
