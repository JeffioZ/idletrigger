package processpicker

import (
	"fmt"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/JeffioZ/idletrigger/internal/automation"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/processcatalog"
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
	if shouldAutoRefreshProcessPicker(time.Time{}, now, false) {
		t.Fatal("an uninitialized picker should not start a second load")
	}
	if shouldAutoRefreshProcessPicker(now.Add(-processPickerRefreshAge), now, true) {
		t.Fatal("an in-flight load should not be restarted")
	}
	if shouldAutoRefreshProcessPicker(now.Add(-processPickerRefreshAge+time.Millisecond), now, false) {
		t.Fatal("a fresh snapshot was refreshed too early")
	}
	if !shouldAutoRefreshProcessPicker(now.Add(-processPickerRefreshAge), now, false) {
		t.Fatal("a stale snapshot was not refreshed after reactivation")
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
