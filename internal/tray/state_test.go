package tray

import (
	"testing"

	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/power"
)

func TestNoSleepRequestSources(t *testing.T) {
	cfg := config.DefaultConfig()
	if noSleepRequested(cfg, false) {
		t.Fatal("unexpected request")
	}
	if !noSleepRequested(cfg, true) {
		t.Fatal("process request was ignored")
	}
	cfg.NoSleepEnabled = true
	if !noSleepRequested(cfg, false) {
		t.Fatal("user request was ignored")
	}
}

func TestBatteryPolicyBlocksAndRestores(t *testing.T) {
	cfg := config.DefaultConfig()
	battery := power.Status{Battery: true, Percent: 80, Valid: true}
	if !batteryPolicyBlocks(cfg, battery) {
		t.Fatal("battery policy should block by default")
	}
	cfg.NoSleepOnBattery = true
	if batteryPolicyBlocks(cfg, battery) {
		t.Fatal("battery policy should allow an adequately charged battery")
	}
	battery.Percent = 10
	if !batteryPolicyBlocks(cfg, battery) {
		t.Fatal("low battery threshold was ignored")
	}
	battery.ACLine = true
	if batteryPolicyBlocks(cfg, battery) {
		t.Fatal("AC power should clear the battery block")
	}
}

func TestStatusLineUsesLocalizedPunctuation(t *testing.T) {
	state := trayState{lang: "zh-CN"}
	if got := state.statusLine("status_power", "交流电源"); got != "电源：交流电源" {
		t.Fatalf("localized status line = %q", got)
	}
}
