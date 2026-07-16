package processpicker

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/automation"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/processcatalog"
	"github.com/JeffioZ/idletrigger/internal/ui/font"
	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
	"github.com/JeffioZ/idletrigger/internal/ui/wintest"
)

var (
	pickerTestUser32         = windows.NewLazySystemDLL("user32.dll")
	pickerTestGetWindowRect  = pickerTestUser32.NewProc("GetWindowRect")
	pickerTestGetClientRect  = pickerTestUser32.NewProc("GetClientRect")
	pickerTestClientToScreen = pickerTestUser32.NewProc("ClientToScreen")
	pickerTestEnumMonitors   = pickerTestUser32.NewProc("EnumDisplayMonitors")
	pickerTestMonitorFromWnd = pickerTestUser32.NewProc("MonitorFromWindow")
	pickerTestGetMonitorInfo = pickerTestUser32.NewProc("GetMonitorInfoW")
	pickerTestScrollBarInfo  = pickerTestUser32.NewProc("GetScrollBarInfo")
)

func TestBuildItemsKeepsRunningListDeduplicatedByName(t *testing.T) {
	path := `C:\Apps\player.exe`
	target := automation.ProcessTarget{Match: automation.MatchPath, Executable: "player.exe", Path: path}
	selected := map[string]automation.ProcessTarget{target.Key(): target}
	groups := []processcatalog.Group{{Executable: "player.exe", Count: 2}}
	items := buildItems(groups, selected, func(key string) string { return key })
	if len(items) != 1 || items[0].target.Match != automation.MatchName || items[0].count != "2" {
		t.Fatalf("items = %+v", items)
	}
}

func TestProcessPickerAutoRefreshUsesStaleActivationOnly(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.Local)
	scanDuration := 20 * time.Millisecond
	refreshAge := processPickerRefreshAge(scanDuration)
	if shouldAutoRefreshProcessPicker(time.Time{}, now, scanDuration, false) {
		t.Fatal("an uninitialized picker should not start a second load")
	}
	if shouldAutoRefreshProcessPicker(now.Add(-refreshAge), now, scanDuration, true) {
		t.Fatal("an in-flight load should not be restarted")
	}
	if shouldAutoRefreshProcessPicker(now.Add(-refreshAge+time.Millisecond), now, scanDuration, false) {
		t.Fatal("a fresh snapshot was refreshed too early")
	}
	if !shouldAutoRefreshProcessPicker(now.Add(-refreshAge), now, scanDuration, false) {
		t.Fatal("a stale snapshot was not refreshed after reactivation")
	}
}

func TestProcessPickerRefreshAgeAdaptsToMeasuredScanCost(t *testing.T) {
	if got := processPickerRefreshAge(0); got != processPickerAutoRefreshMinAge {
		t.Fatalf("zero-cost refresh age = %v, want %v", got, processPickerAutoRefreshMinAge)
	}
	if got := processPickerRefreshAge(100 * time.Millisecond); got != 25*time.Second {
		t.Fatalf("100 ms refresh age = %v, want 25s", got)
	}
	if got := processPickerRefreshAge(time.Second); got != processPickerAutoRefreshMaxAge {
		t.Fatalf("slow refresh age = %v, want %v", got, processPickerAutoRefreshMaxAge)
	}
}

func TestDescriptionCandidatesSkipPositiveAndNegativeCacheUnlessManual(t *testing.T) {
	instances := []processcatalog.Instance{
		{PID: 1, Executable: "cached.exe"},
		{PID: 2, Executable: "missing.exe"},
		{PID: 3, Executable: "new.exe"},
		{PID: 4, Executable: "new.exe"},
	}
	key := func(name string) string {
		return (automation.ProcessTarget{Match: automation.MatchName, Executable: name}).Key()
	}
	cached := map[string]string{key("cached.exe"): "Cached"}
	attempted := map[string]struct{}{key("missing.exe"): {}}
	candidates, attempts := descriptionCandidates(instances, cached, attempted, false)
	if len(candidates) != 2 || len(attempts) != 1 || attempts[0] != key("new.exe") {
		t.Fatalf("automatic candidates=%+v attempts=%+v", candidates, attempts)
	}
	candidates, attempts = descriptionCandidates(instances, cached, attempted, true)
	if len(candidates) != 3 || len(attempts) != 2 {
		t.Fatalf("manual candidates=%+v attempts=%+v", candidates, attempts)
	}
}

func TestFilterMatchesNameOrDescription(t *testing.T) {
	items := []item{
		{name: "player.exe", description: "Media Player", search: "player.exe media player"},
		{name: "render.exe", description: "Renderer", search: "render.exe renderer"},
	}
	got := filterItems(items, "media")
	if len(got) != 1 || got[0].name != "player.exe" {
		t.Fatalf("filtered items = %+v", got)
	}
}

func TestSameRelativeTargetOrder(t *testing.T) {
	values := func(names ...string) []item {
		items := make([]item, 0, len(names))
		for _, name := range names {
			items = append(items, item{target: automation.ProcessTarget{Match: automation.MatchName, Executable: name}})
		}
		return items
	}
	if !sameRelativeTargetOrder(values("a.exe", "b.exe", "c.exe"), values("a.exe", "c.exe")) {
		t.Fatal("filtering should preserve the relative row order")
	}
	if !sameRelativeTargetOrder(values("a.exe", "c.exe"), values("a.exe", "b.exe", "c.exe")) {
		t.Fatal("clearing a filter should preserve the relative row order")
	}
	if sameRelativeTargetOrder(values("a.exe", "b.exe", "c.exe"), values("c.exe", "b.exe", "a.exe")) {
		t.Fatal("sorting should require an ordered rebuild")
	}
	if sameRelativeTargetOrder(values("a.exe"), values("b.exe")) {
		t.Fatal("a disjoint refresh should use the fast full rebuild")
	}
}

func TestProcessColumnWidthsNeverRequireHorizontalScrolling(t *testing.T) {
	for _, test := range []struct {
		clientWidth int
		scale       float64
	}{
		{clientWidth: 996, scale: 1},
		{clientWidth: 1245, scale: 1.25},
		{clientWidth: 40, scale: 1},
	} {
		widths := processColumnWidths(test.clientWidth, test.scale)
		total := widths[0] + widths[1] + widths[2]
		if total > test.clientWidth {
			t.Fatalf("processColumnWidths(%d, %.2f) = %v (total %d)", test.clientWidth, test.scale, widths, total)
		}
		for _, width := range widths {
			if width < 0 {
				t.Fatalf("processColumnWidths(%d, %.2f) contains a negative width: %v", test.clientWidth, test.scale, widths)
			}
		}
	}

	wide := processColumnWidths(996, 1)
	if wide[1] != 310 || wide[0] <= wide[1] {
		t.Fatalf("wide process picker should keep description compact and give spare room to process names: %v", wide)
	}
}

func TestWriteUTF16TextTerminatesAndTruncates(t *testing.T) {
	buffer := make([]uint16, 5)
	writeUTF16Text(&buffer[0], int32(len(buffer)), "进程说明")
	if buffer[len(buffer)-1] != 0 {
		t.Fatalf("buffer is not terminated: %v", buffer)
	}
	got := string(utf16.Decode(buffer[:len(buffer)-1]))
	if got != "进程说明" {
		t.Fatalf("decoded text = %q, want %q", got, "进程说明")
	}

	short := make([]uint16, 4)
	writeUTF16Text(&short[0], int32(len(short)), "abcdef")
	if got := string(utf16.Decode(short[:3])); got != "abc" || short[3] != 0 {
		t.Fatalf("truncated buffer = %v, decoded = %q", short, got)
	}
}

func TestSelectionLimitRejectsOnlyTheAdditionalTarget(t *testing.T) {
	selected := make(map[string]automation.ProcessTarget, automation.MaxProcessesPerRule)
	for index := 0; index < automation.MaxProcessesPerRule; index++ {
		target := automation.ProcessTarget{Match: automation.MatchName, Executable: fmt.Sprintf("process-%02d.exe", index)}
		selected[target.Key()] = target
	}
	existing := selected["name:process-00.exe"]
	if !canAddSelection(selected, existing) {
		t.Fatal("an already-selected target should remain selectable at the limit")
	}
	additional := automation.ProcessTarget{Match: automation.MatchName, Executable: "additional.exe"}
	if canAddSelection(selected, additional) {
		t.Fatal("a 65th process target was accepted")
	}
	pathTarget := automation.ProcessTarget{Match: automation.MatchPath, Executable: "process-00.exe", Path: `C:\Apps\process-00.exe`}
	delete(selected, existing.Key())
	selected[pathTarget.Key()] = pathTarget
	if !canAddSelection(selected, existing) {
		t.Fatal("a name target that replaces an exact-path target should be allowed at the limit")
	}
	if got := normalizeSelected(selected); len(got) != automation.MaxProcessesPerRule {
		t.Fatalf("normalization changed the existing selection: %d", len(got))
	}
}

func TestProcessPickerRemainsOperableAcrossDPIAndSmallWorkArea(t *testing.T) {
	work := nativeform.Rect{Right: 1366, Bottom: 768}
	groups := []processcatalog.Group{{Executable: "player.exe", Description: "Media Player", Count: 2}}
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		t.Run(formatPickerScale(scale), func(t *testing.T) {
			err := Capture(testPickerOptions(), groups, scale, false, func(hwnd windows.Handle) error {
				p := activePickerForTest(t, hwnd)
				p.positionInWorkArea(&work)
				if p.layoutErr != nil {
					t.Fatalf("small-work-area layout: %v", p.layoutErr)
				}
				assertPickerRectInside(t, pickerWindowRect(t, hwnd), work)
				p.scrollContentTo(max(0, windowHeight-p.viewportHeight))
				client := pickerClientScreenRect(t, hwnd)
				for _, id := range []uint16{idCancel, idConfirm} {
					assertPickerRectInside(t, pickerWindowRect(t, p.controls[id]), client)
				}
				assertNoHorizontalProcessScroll(t, p)
				assertHeaderReachesListEdge(t, p)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProcessPickerAppliesSuggestedRectAcrossDPIChanges(t *testing.T) {
	err := Capture(testPickerOptions(), nil, 1, false, func(hwnd windows.Handle) error {
		p := activePickerForTest(t, hwnd)
		p.captureScale = 0
		workAreas := pickerMonitorWorkAreas(t)
		for index, dpi := range []uint32{96, 120, 144, 192, 120} {
			work := workAreas[index%len(workAreas)]
			scale := float64(dpi) / 96
			width, height, err := nativeform.WindowSizeForClient(int(windowWidth*scale+0.5), int(windowHeight*scale+0.5), p.style, p.exStyle, dpi)
			if err != nil {
				t.Fatal(err)
			}
			suggested := nativeform.Rect{Left: work.Left + 19, Top: work.Top + 23, Right: work.Left + 19 + width, Bottom: work.Top + 23 + height}
			pSendMessage.Call(uintptr(hwnd), wmDpiChanged, uintptr(dpi|(dpi<<16)), uintptr(unsafe.Pointer(&suggested)))
			if p.dpiScale != scale {
				t.Fatalf("DPI scale = %.2f, want %.2f", p.dpiScale, scale)
			}
			want := nativeform.ConstrainRect(suggested, work)
			if got := pickerWindowRect(t, hwnd); got != want {
				t.Fatalf("window rect after %d DPI = %+v, want %+v", dpi, got, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProcessPickerReleasesResourcesAcrossRepresentativeCycles(t *testing.T) {
	const (
		stabilizationCycles = 8
		measuredCycles      = 8
	)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	groups := []processcatalog.Group{{Executable: "player.exe", Description: "Media Player", Count: 2}}
	// A native dark-mode ListView lazily loads process-wide common-control
	// caches. Warm both themes before measuring a representative second batch.
	for index := 0; index < stabilizationCycles; index++ {
		if err := Capture(testPickerOptions(), groups, 1, index%2 != 0, nil); err != nil {
			t.Fatalf("stabilization cycle %d: %v", index+1, err)
		}
	}
	before, err := wintest.StableResources()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < measuredCycles; index++ {
		if err := Capture(testPickerOptions(), groups, 1, index%2 == 0, nil); err != nil {
			t.Fatalf("cycle %d: %v", index+1, err)
		}
	}
	after, err := wintest.StableResources()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("process-picker resources across %d cycles before=%+v after=%+v", measuredCycles, before, after)
	if after.GDI > before.GDI || after.USER > before.USER {
		t.Fatalf("GUI resources grew after %d process-picker cycles: before=%+v after=%+v", measuredCycles, before, after)
	}
}

func TestProcessPickerCreationFailuresReleasePartialResources(t *testing.T) {
	if err := Capture(testPickerOptions(), nil, 1, false, nil); err != nil {
		t.Fatal(err)
	}
	type fault struct {
		name    string
		install func() func()
	}
	createFailure := func(failAt int) func() func() {
		return func() func() {
			original := createWindowForPicker
			calls := 0
			createWindowForPicker = func(exStyle uintptr, class, caption *uint16, style uintptr, x, y, width, height int, parent windows.Handle, id uintptr) (windows.Handle, error) {
				calls++
				if calls == failAt {
					return 0, errors.New("injected CreateWindowEx failure")
				}
				return original(exStyle, class, caption, style, x, y, width, height, parent, id)
			}
			return func() { createWindowForPicker = original }
		}
	}
	faults := []fault{
		{"top-level HWND", createFailure(1)},
		{"first child HWND", createFailure(2)},
		{"list HWND", createFailure(9)},
		{"tooltip HWND", createFailure(18)},
		{"font", func() func() {
			original := newFontForPicker
			newFontForPicker = func(int32, int32, bool) (windows.Handle, font.Choice) { return 0, font.Choice{} }
			return func() { newFontForPicker = original }
		}},
		{"brush", func() func() {
			original := createBrushForPicker
			calls := 0
			createBrushForPicker = func(color uint32) windows.Handle {
				calls++
				if calls == 1 {
					return 0
				}
				return original(color)
			}
			return func() { createBrushForPicker = original }
		}},
		{"cue banner", func() func() {
			original := newCueBannerForPicker
			newCueBannerForPicker = func(windows.Handle, string, uint32, float64) (*nativeform.CueBanner, error) {
				return nil, errors.New("injected cue failure")
			}
			return func() { newCueBannerForPicker = original }
		}},
		{"list scrollbar", func() func() {
			original := newScrollbarForPicker
			newScrollbarForPicker = func(nativeform.ScrollbarOptions) (*nativeform.Scrollbar, error) {
				return nil, errors.New("injected scrollbar failure")
			}
			return func() { newScrollbarForPicker = original }
		}},
		{"preview scrollbar", func() func() {
			original := newListboxScrollForPicker
			newListboxScrollForPicker = func(nativeform.ListboxScrollbarOptions) (*nativeform.ListboxScrollbar, error) {
				return nil, errors.New("injected listbox scrollbar failure")
			}
			return func() { newListboxScrollForPicker = original }
		}},
		{"content scrollbar", func() func() {
			original := newScrollbarForPicker
			calls := 0
			newScrollbarForPicker = func(options nativeform.ScrollbarOptions) (*nativeform.Scrollbar, error) {
				calls++
				if calls == 2 {
					return nil, errors.New("injected content scrollbar failure")
				}
				return original(options)
			}
			return func() { newScrollbarForPicker = original }
		}},
	}
	for _, test := range faults {
		t.Run(test.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			if err := Capture(testPickerOptions(), nil, 1, false, nil); err != nil {
				t.Fatal(err)
			}
			before, err := wintest.StableResources()
			if err != nil {
				t.Fatal(err)
			}
			restore := test.install()
			err = Capture(testPickerOptions(), nil, 1, false, nil)
			restore()
			if err == nil {
				t.Fatal("injected creation failure reported success")
			}
			activeMu.Lock()
			remaining := active
			activeMu.Unlock()
			if remaining != nil {
				t.Fatalf("active picker survived creation failure: %+v", remaining)
			}
			after, snapshotErr := wintest.StableResources()
			if snapshotErr != nil {
				t.Fatal(snapshotErr)
			}
			if after.GDI != before.GDI || after.USER != before.USER {
				t.Fatalf("GUI resources changed after injected failure: before=%+v after=%+v", before, after)
			}
			t.Logf("injected failure resources before=%+v after=%+v", before, after)
		})
	}
}

func TestProcessPickerIgnoresCallbacksThatArriveAfterClose(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	originalSnapshot := snapshotNamesForPicker
	originalEnrich := enrichDescriptionsForPicker
	originalPost := postPickerUI
	defer func() {
		snapshotNamesForPicker = originalSnapshot
		enrichDescriptionsForPicker = originalEnrich
		postPickerUI = originalPost
	}()
	started := make(chan struct{})
	release := make(chan struct{})
	snapshotNamesForPicker = func() ([]processcatalog.Instance, error) {
		close(started)
		<-release
		return []processcatalog.Instance{{Executable: "late.exe"}}, nil
	}
	enrichDescriptionsForPicker = func(values []processcatalog.Instance) []processcatalog.Instance { return values }
	var callbacks []func()
	var callbackMu sync.Mutex
	postPickerUI = func(callback func()) bool {
		callbackMu.Lock()
		callbacks = append(callbacks, callback)
		callbackMu.Unlock()
		return true
	}
	if err := Show(testPickerOptions()); err != nil {
		t.Fatal(err)
	}
	activeMu.Lock()
	p := active
	activeMu.Unlock()
	if p == nil {
		t.Fatal("picker was not active")
	}
	<-started
	Hide()
	close(release)
	p.workers.Wait()
	callbackMu.Lock()
	late := append([]func(){}, callbacks...)
	callbackMu.Unlock()
	for _, callback := range late {
		callback()
	}
	if !p.closed.Load() || p.hwnd != 0 {
		t.Fatalf("closed picker retained live state: closed=%v hwnd=%v", p.closed.Load(), p.hwnd)
	}
	activeMu.Lock()
	remaining := active
	activeMu.Unlock()
	if remaining != nil {
		t.Fatalf("late callback restored active picker: %+v", remaining)
	}
	if len(p.items) != 0 {
		t.Fatalf("late callback rewrote closed picker items: %+v", p.items)
	}
}

func testPickerOptions() Options {
	return Options{Text: func(key string) string { return key }}
}

func activePickerForTest(t *testing.T, hwnd windows.Handle) *picker {
	t.Helper()
	activeMu.Lock()
	p := active
	activeMu.Unlock()
	if p == nil || p.hwnd != hwnd {
		t.Fatalf("active process picker does not match hwnd %v", hwnd)
	}
	return p
}

func formatPickerScale(scale float64) string {
	return map[float64]string{1: "100", 1.25: "125", 1.5: "150", 2: "200"}[scale]
}

func pickerWindowRect(t *testing.T, hwnd windows.Handle) nativeform.Rect {
	t.Helper()
	var value nativeform.Rect
	if ok, _, callErr := pickerTestGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&value))); ok == 0 {
		t.Fatalf("GetWindowRect(%v): %v", hwnd, callErr)
	}
	return value
}

func pickerClientScreenRect(t *testing.T, hwnd windows.Handle) nativeform.Rect {
	t.Helper()
	var value nativeform.Rect
	if ok, _, callErr := pickerTestGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&value))); ok == 0 {
		t.Fatalf("GetClientRect(%v): %v", hwnd, callErr)
	}
	topLeft := struct{ X, Y int32 }{X: value.Left, Y: value.Top}
	bottomRight := struct{ X, Y int32 }{X: value.Right, Y: value.Bottom}
	pickerTestClientToScreen.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&topLeft)))
	pickerTestClientToScreen.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&bottomRight)))
	return nativeform.Rect{Left: topLeft.X, Top: topLeft.Y, Right: bottomRight.X, Bottom: bottomRight.Y}
}

func pickerMonitorWorkArea(t *testing.T, hwnd windows.Handle) nativeform.Rect {
	t.Helper()
	monitor, _, _ := pickerTestMonitorFromWnd.Call(uintptr(hwnd), 2)
	if monitor == 0 {
		t.Fatal("MonitorFromWindow returned zero")
	}
	type info struct {
		Size          uint32
		Monitor, Work nativeform.Rect
		Flags         uint32
	}
	value := info{Size: uint32(unsafe.Sizeof(info{}))}
	if ok, _, callErr := pickerTestGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&value))); ok == 0 {
		t.Fatalf("GetMonitorInfo: %v", callErr)
	}
	return value.Work
}

func pickerMonitorWorkAreas(t *testing.T) []nativeform.Rect {
	t.Helper()
	var values []nativeform.Rect
	callback := windows.NewCallback(func(monitor, hdc, clip, data uintptr) uintptr {
		type info struct {
			Size          uint32
			Monitor, Work nativeform.Rect
			Flags         uint32
		}
		value := info{Size: uint32(unsafe.Sizeof(info{}))}
		if ok, _, _ := pickerTestGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&value))); ok != 0 {
			values = append(values, value.Work)
		}
		return 1
	})
	if ok, _, callErr := pickerTestEnumMonitors.Call(0, 0, callback, 0); ok == 0 {
		t.Fatalf("EnumDisplayMonitors: %v", callErr)
	}
	if len(values) == 0 {
		t.Fatal("EnumDisplayMonitors returned no work areas")
	}
	t.Logf("exercising WM_DPICHANGED across %d monitor work area(s)", len(values))
	return values
}

func assertPickerRectInside(t *testing.T, inner, outer nativeform.Rect) {
	t.Helper()
	if inner.Left < outer.Left || inner.Top < outer.Top || inner.Right > outer.Right || inner.Bottom > outer.Bottom {
		t.Fatalf("rect %+v escapes %+v", inner, outer)
	}
}

func assertNoHorizontalProcessScroll(t *testing.T, p *picker) {
	t.Helper()
	type scrollBarInfo struct {
		Size                        uint32
		Scroll                      nativeform.Rect
		Line, ThumbTop, ThumbBottom int32
		Reserved                    int32
		State                       [6]uint32
	}
	value := scrollBarInfo{Size: uint32(unsafe.Sizeof(scrollBarInfo{}))}
	const (
		objectHorizontalScroll = ^uintptr(5) // OBJID_HSCROLL (-6)
		stateInvisible         = 0x00008000
	)
	ok, _, _ := pickerTestScrollBarInfo.Call(uintptr(p.controls[idList]), objectHorizontalScroll, uintptr(unsafe.Pointer(&value)))
	if ok != 0 && value.State[0]&stateInvisible == 0 {
		t.Fatalf("process list horizontal scrollbar is visible: %+v", value)
	}
}

func assertHeaderReachesListEdge(t *testing.T, p *picker) {
	t.Helper()
	var headerClient, last rect
	if ok, _, _ := pGetClientRect.Call(uintptr(p.header), uintptr(unsafe.Pointer(&headerClient))); ok == 0 {
		t.Fatal("read process header client bounds")
	}
	if ok, _, _ := pSendMessage.Call(uintptr(p.header), hdmGetItemRect, 2, uintptr(unsafe.Pointer(&last))); ok == 0 {
		t.Fatal("read last process header item")
	}
	if gap := headerClient.Right - last.Right; gap < 0 || gap > 1 {
		t.Fatalf("process header right remainder = %d px (client=%+v last=%+v)", gap, headerClient, last)
	}
}
