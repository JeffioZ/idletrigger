package automationpanel

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/automation"
	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
	"github.com/JeffioZ/idletrigger/internal/ui/wintest"
)

var (
	automationTestUser32         = windows.NewLazySystemDLL("user32.dll")
	automationTestGetWindowRect  = automationTestUser32.NewProc("GetWindowRect")
	automationTestGetClientRect  = automationTestUser32.NewProc("GetClientRect")
	automationTestClientToScreen = automationTestUser32.NewProc("ClientToScreen")
	automationTestEnumMonitors   = automationTestUser32.NewProc("EnumDisplayMonitors")
	automationTestMonitorFromWnd = automationTestUser32.NewProc("MonitorFromWindow")
	automationTestGetMonitorInfo = automationTestUser32.NewProc("GetMonitorInfoW")
)

func TestChoiceFocusNotificationDoesNotClosePopup(t *testing.T) {
	const bnKillFocus = 7
	p := &panel{
		choices:    map[uint16]*choiceField{idAction: {}},
		choiceOpen: idAction,
	}
	p.handleEditor(idAction, bnKillFocus)
	if p.choiceOpen != idAction {
		t.Fatal("a choice button focus notification closed the popup")
	}
}

func TestSystemActionOffersProcessLifecycleTriggers(t *testing.T) {
	p := &panel{
		choices: map[uint16]*choiceField{},
		text:    func(key string) string { return key },
	}
	p.setTriggerOptions(automation.ActionShutdown, automation.TriggerProcessStarted)

	if got := p.triggerValue(); got != automation.TriggerProcessStarted {
		t.Fatalf("selected trigger = %q, want %q", got, automation.TriggerProcessStarted)
	}
	want := []automation.Trigger{
		automation.TriggerOnce,
		automation.TriggerDaily,
		automation.TriggerWeekly,
		automation.TriggerProcessStarted,
		automation.TriggerProcessExited,
	}
	if len(p.triggerOptions) != len(want) {
		t.Fatalf("trigger count = %d, want %d", len(p.triggerOptions), len(want))
	}
	for index := range want {
		if p.triggerOptions[index] != want[index] {
			t.Fatalf("trigger[%d] = %q, want %q", index, p.triggerOptions[index], want[index])
		}
	}
}

func TestManagerUsesAuthoritativeStateWhenSaveIsRejected(t *testing.T) {
	latest := State{Revision: "new", Rules: []automation.Rule{{ID: "latest"}}}
	p := &panel{
		view:   managerView,
		state:  State{Revision: "old", Rules: []automation.Rule{{ID: "old"}}},
		rules:  []automation.Rule{{ID: "old"}},
		onSave: func(SaveRequest) SaveResult { return SaveResult{State: latest, Error: "conflict"} },
	}
	if ok, _ := p.notifySave([]automation.Rule{{ID: "stale"}}); ok {
		t.Fatal("rejected save reported success")
	}
	if p.state.Revision != "new" || len(p.rules) != 1 || p.rules[0].ID != "latest" || p.managerNotice != "conflict" {
		t.Fatalf("manager state = %+v, rules = %+v", p.state, p.rules)
	}
}

func TestEditorKeepsDraftBaseWhenExternalStateArrives(t *testing.T) {
	latest := State{Revision: "new", Rules: []automation.Rule{{ID: "latest"}}}
	p := &panel{
		view:   editorView,
		state:  State{Revision: "old", Rules: []automation.Rule{{ID: "old"}}},
		rules:  []automation.Rule{{ID: "old"}},
		onSave: func(SaveRequest) SaveResult { return SaveResult{State: latest, Error: "conflict"} },
	}
	if ok, _ := p.notifySave([]automation.Rule{{ID: "draft"}}); ok {
		t.Fatal("rejected save reported success")
	}
	if p.state.Revision != "old" || p.pendingState == nil || p.pendingState.Revision != "new" {
		t.Fatalf("editor state = %+v, pending = %+v", p.state, p.pendingState)
	}
}

func TestAutomationWindowsRemainOperableAcrossDPIAndSmallWorkArea(t *testing.T) {
	work := nativeform.Rect{Right: 1366, Bottom: 768}
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		for _, editor := range []bool{false, true} {
			name := "manager"
			controls := []uint16{idNew, idEdit, idDelete, idToggle}
			if editor {
				name = "editor"
				controls = []uint16{idCancel, idSave}
			}
			t.Run(name+"-"+formatTestScale(scale), func(t *testing.T) {
				err := Capture(State{}, func(key string) string { return key }, scale, false, editor, func(hwnd windows.Handle) error {
					p := activePanelForTest(t, hwnd)
					p.resizeInWorkArea(p.clientWidth, p.clientHeight, &work)
					if p.layoutErr != nil {
						t.Fatalf("small-work-area layout: %v", p.layoutErr)
					}
					assertRectInside(t, windowRectForTest(t, hwnd), work)
					p.scrollContentTo(max(0, p.clientHeight-p.viewportHeight))
					client := clientScreenRectForTest(t, hwnd)
					for _, id := range controls {
						assertRectInside(t, windowRectForTest(t, p.controls[id]), client)
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestAutomationWindowsApplySuggestedRectAcrossDPIChanges(t *testing.T) {
	for _, editor := range []bool{false, true} {
		t.Run(map[bool]string{false: "manager", true: "editor"}[editor], func(t *testing.T) {
			err := Capture(State{}, func(key string) string { return key }, 1, false, editor, func(hwnd windows.Handle) error {
				p := activePanelForTest(t, hwnd)
				p.captureScale = 0
				workAreas := monitorWorkAreasForTest(t)
				for index, dpi := range []uint32{96, 120, 144, 192, 120} {
					work := workAreas[index%len(workAreas)]
					scale := float64(dpi) / 96
					width, height, err := nativeform.WindowSizeForClient(
						int(float64(p.clientWidth)*scale+0.5), int(float64(p.clientHeight)*scale+0.5),
						p.style, p.exStyle, dpi,
					)
					if err != nil {
						t.Fatal(err)
					}
					suggested := nativeform.Rect{Left: work.Left + 13, Top: work.Top + 17, Right: work.Left + 13 + width, Bottom: work.Top + 17 + height}
					pSendMessage.Call(uintptr(hwnd), wmDpiChanged, uintptr(dpi|(dpi<<16)), uintptr(unsafe.Pointer(&suggested)))
					if p.dpiScale != scale {
						t.Fatalf("DPI scale = %.2f, want %.2f", p.dpiScale, scale)
					}
					want := nativeform.ConstrainRect(suggested, work)
					if got := windowRectForTest(t, hwnd); got != want {
						t.Fatalf("window rect after %d DPI = %+v, want suggested/clamped %+v", dpi, got, want)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAutomationWindowsReleaseResourcesAfter100OpenCloseCycles(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	text := func(key string) string { return key }
	for range 3 {
		if err := Capture(State{}, text, 1, false, true, nil); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	before, err := wintest.SnapshotResources()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 100; index++ {
		if err := Capture(State{}, text, 1, index%2 == 0, index%2 != 0, nil); err != nil {
			t.Fatalf("cycle %d: %v", index+1, err)
		}
	}
	runtime.GC()
	after, err := wintest.SnapshotResources()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("automation resources before=%+v after=%+v", before, after)
	if after.GDI > before.GDI || after.USER > before.USER || after.Handles > before.Handles || after.Threads > before.Threads {
		t.Fatalf("resources grew after 100 automation-window cycles: before=%+v after=%+v", before, after)
	}
}

func activePanelForTest(t *testing.T, hwnd windows.Handle) *panel {
	t.Helper()
	activeMu.Lock()
	p := active
	activeMu.Unlock()
	if p == nil || p.hwnd != hwnd {
		t.Fatalf("active automation panel = %p hwnd=%v, want hwnd=%v", p, func() windows.Handle {
			if p != nil {
				return p.hwnd
			}
			return 0
		}(), hwnd)
	}
	return p
}

func formatTestScale(scale float64) string {
	return map[float64]string{1: "100", 1.25: "125", 1.5: "150", 2: "200"}[scale]
}

func windowRectForTest(t *testing.T, hwnd windows.Handle) nativeform.Rect {
	t.Helper()
	var value nativeform.Rect
	if ok, _, callErr := automationTestGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&value))); ok == 0 {
		t.Fatalf("GetWindowRect(%v): %v", hwnd, callErr)
	}
	return value
}

func clientScreenRectForTest(t *testing.T, hwnd windows.Handle) nativeform.Rect {
	t.Helper()
	var value nativeform.Rect
	if ok, _, callErr := automationTestGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&value))); ok == 0 {
		t.Fatalf("GetClientRect(%v): %v", hwnd, callErr)
	}
	topLeft := point{X: value.Left, Y: value.Top}
	bottomRight := point{X: value.Right, Y: value.Bottom}
	automationTestClientToScreen.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&topLeft)))
	automationTestClientToScreen.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&bottomRight)))
	return nativeform.Rect{Left: topLeft.X, Top: topLeft.Y, Right: bottomRight.X, Bottom: bottomRight.Y}
}

func monitorWorkAreaForTest(t *testing.T, hwnd windows.Handle) nativeform.Rect {
	t.Helper()
	monitor, _, _ := automationTestMonitorFromWnd.Call(uintptr(hwnd), 2)
	if monitor == 0 {
		t.Fatal("MonitorFromWindow returned zero")
	}
	type info struct {
		Size          uint32
		Monitor, Work nativeform.Rect
		Flags         uint32
	}
	value := info{Size: uint32(unsafe.Sizeof(info{}))}
	if ok, _, callErr := automationTestGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&value))); ok == 0 {
		t.Fatalf("GetMonitorInfo: %v", callErr)
	}
	return value.Work
}

func monitorWorkAreasForTest(t *testing.T) []nativeform.Rect {
	t.Helper()
	var values []nativeform.Rect
	callback := windows.NewCallback(func(monitor, hdc, clip, data uintptr) uintptr {
		type info struct {
			Size          uint32
			Monitor, Work nativeform.Rect
			Flags         uint32
		}
		value := info{Size: uint32(unsafe.Sizeof(info{}))}
		if ok, _, _ := automationTestGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&value))); ok != 0 {
			values = append(values, value.Work)
		}
		return 1
	})
	if ok, _, callErr := automationTestEnumMonitors.Call(0, 0, callback, 0); ok == 0 {
		t.Fatalf("EnumDisplayMonitors: %v", callErr)
	}
	if len(values) == 0 {
		t.Fatal("EnumDisplayMonitors returned no work areas")
	}
	t.Logf("exercising WM_DPICHANGED across %d monitor work area(s)", len(values))
	return values
}

func assertRectInside(t *testing.T, inner, outer nativeform.Rect) {
	t.Helper()
	if inner.Left < outer.Left || inner.Top < outer.Top || inner.Right > outer.Right || inner.Bottom > outer.Bottom {
		t.Fatalf("rect %+v escapes %+v", inner, outer)
	}
}
