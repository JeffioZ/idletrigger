package processpicker

import (
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
)

const (
	headerListSubclassID = 0x49544844
	headerMouseWheel     = 0x020A
	headerVScroll        = 0x0115
	headerKeyDown        = 0x0100
	hdmFirst             = 0x1200
	hdmGetItemRect       = hdmFirst + 7
)

var (
	headerComctl32         = windows.NewLazySystemDLL("comctl32.dll")
	pSetWindowSubclass     = headerComctl32.NewProc("SetWindowSubclass")
	pRemoveWindowSubclass  = headerComctl32.NewProc("RemoveWindowSubclass")
	pDefSubclassProc       = headerComctl32.NewProc("DefSubclassProc")
	headerSubclassCallback = windows.NewCallback(headerListSubclassProc)
)

// installHeaderRenderer keeps the native Explorer-themed header. Native
// headers already provide reliable hover, pressed, keyboard and high-contrast
// states; replacing their WM_PAINT path made first-frame visibility dependent
// on a later mouse invalidation. Only the list subclass remains, to keep the
// themed overlay scrollbar synchronized with native scrolling.
func (p *picker) installHeaderRenderer() error {
	list := p.controls[idList]
	if list == 0 {
		return fmt.Errorf("process picker list is required")
	}
	if ok, _, callErr := pSetWindowSubclass.Call(uintptr(list), headerSubclassCallback, headerListSubclassID, 0); ok == 0 {
		return fmt.Errorf("subclass process picker list: %w", callErr)
	}
	header, _, _ := pSendMessage.Call(uintptr(list), lvmGetHeader, 0, 0)
	if header == 0 {
		pRemoveWindowSubclass.Call(uintptr(list), headerSubclassCallback, headerListSubclassID)
		return fmt.Errorf("resolve process picker header")
	}
	p.header = windows.Handle(header)
	nativeform.ApplyControl(p.header, p.themeDark)
	nativeform.PresentControl(p.header, true)
	return nil
}

func (p *picker) releaseHeaderRenderer() {
	if p.controls != nil && p.controls[idList] != 0 {
		pRemoveWindowSubclass.Call(uintptr(p.controls[idList]), headerSubclassCallback, headerListSubclassID)
	}
	p.header = 0
}

func headerListSubclassProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr, subclassID, refData uintptr) uintptr {
	if message == headerMouseWheel {
		activeMu.Lock()
		p := active
		activeMu.Unlock()
		if p != nil && hwnd == p.controls[idList] && p.scrollListWheel(wParam) {
			return 0
		}
	}
	result, _, _ := pDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	switch message {
	case headerVScroll, headerKeyDown:
		activeMu.Lock()
		p := active
		activeMu.Unlock()
		if p != nil && hwnd == p.controls[idList] {
			p.syncListScrollbar()
		}
	}
	return result
}
