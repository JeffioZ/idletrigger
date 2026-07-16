package processpicker

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
)

type nmCustomDraw struct {
	Header    nmHeader
	DrawStage uint32
	HDC       windows.Handle
	Rect      rect
	ItemSpec  uintptr
	ItemState uint32
	ItemParam uintptr
}

type headerHitTest struct {
	Point struct{ X, Y int32 }
	Flags uint32
	Item  int32
}

type headerTrackMouse struct {
	Size      uint32
	Flags     uint32
	Track     windows.Handle
	HoverTime uint32
}

const (
	wmHeaderNotify       = 0x004E
	wmHeaderNCDestroy    = 0x0082
	wmHeaderCancelMode   = 0x001F
	wmHeaderKeyDown      = 0x0100
	wmHeaderVScroll      = 0x0115
	wmHeaderMouseMove    = 0x0200
	wmHeaderLButtonDown  = 0x0201
	wmHeaderLButtonUp    = 0x0202
	wmHeaderMouseWheel   = 0x020A
	wmHeaderMouseLeave   = 0x02A3
	wmHeaderCaptureLost  = 0x0215
	nmCustomDrawCode     = -12
	cdStagePrePaint      = 0x00000001
	cdStagePostPaint     = 0x00000002
	cdStageItem          = 0x00010000
	cdStateSelected      = 0x00000001
	cdStateHot           = 0x00000040
	cdResultSkipDefault  = 0x00000004
	cdResultNotifyPost   = 0x00000010
	cdResultNotifyItems  = 0x00000020
	headerListSubclassID = 0x49544844
	headerSubclassID     = 0x49544845
	hdmFirst             = 0x1200
	hdmHitTest           = hdmFirst + 6
	hdmGetItemRect       = hdmFirst + 7
	hdmGetItemCount      = hdmFirst + 0
	headerTMELeave       = 0x00000002
)

var (
	headerComctl32         = windows.NewLazySystemDLL("comctl32.dll")
	pSetWindowSubclass     = headerComctl32.NewProc("SetWindowSubclass")
	pRemoveWindowSubclass  = headerComctl32.NewProc("RemoveWindowSubclass")
	pDefSubclassProc       = headerComctl32.NewProc("DefSubclassProc")
	pHeaderTrackMouse      = windows.NewLazySystemDLL("user32.dll").NewProc("TrackMouseEvent")
	headerSubclassCallback = windows.NewCallback(headerSubclassProc)
)

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
	p.headerHover, p.headerPressed = -1, -1
	if ok, _, callErr := pSetWindowSubclass.Call(header, headerSubclassCallback, headerSubclassID, 0); ok == 0 {
		pRemoveWindowSubclass.Call(uintptr(list), headerSubclassCallback, headerListSubclassID)
		p.header = 0
		return fmt.Errorf("subclass process picker header: %w", callErr)
	}
	pInvalidateRect.Call(header, 0, 0)
	return nil
}

func (p *picker) releaseHeaderRenderer() {
	if p.header != 0 {
		pRemoveWindowSubclass.Call(uintptr(p.header), headerSubclassCallback, headerSubclassID)
	}
	if p.controls != nil && p.controls[idList] != 0 {
		pRemoveWindowSubclass.Call(uintptr(p.controls[idList]), headerSubclassCallback, headerListSubclassID)
	}
	p.header, p.headerHover, p.headerPressed = 0, -1, -1
}

func (p *picker) headerItemAt(hwnd windows.Handle, lParam uintptr) int {
	value := lParam
	hit := headerHitTest{Item: -1}
	hit.Point.X = int32(int16(value))
	hit.Point.Y = int32(int16(value >> 16))
	result, _, _ := pSendMessage.Call(uintptr(hwnd), hdmHitTest, 0, uintptr(unsafe.Pointer(&hit)))
	if int32(result) < 0 {
		return -1
	}
	return int(hit.Item)
}

func (p *picker) invalidateHeader() {
	if p.header != 0 {
		pInvalidateRect.Call(uintptr(p.header), 0, 0)
	}
}

func headerSubclassProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr, subclassID, refData uintptr) uintptr {
	activeMu.Lock()
	p := active
	activeMu.Unlock()
	if p == nil {
		result, _, _ := pDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		return result
	}
	if hwnd == p.controls[idList] && message == wmHeaderNotify && lParam != 0 {
		draw := (*nmCustomDraw)(nativeform.MessagePointer(lParam))
		if draw.Header.Code == nmCustomDrawCode {
			switch draw.DrawStage {
			case cdStagePrePaint:
				return cdResultNotifyItems | cdResultNotifyPost
			case cdStagePrePaint | cdStageItem:
				index := int(draw.ItemSpec)
				scale := int32(p.scale() + 0.5)
				if scale < 1 {
					scale = 1
				}
				bounds := nativeform.Rect{Left: draw.Rect.Left, Top: draw.Rect.Top, Right: draw.Rect.Right, Bottom: draw.Rect.Bottom}
				state := nativeform.ControlState{
					Hovered: index == p.headerHover || draw.ItemState&cdStateHot != 0,
					Pressed: index == p.headerPressed || draw.ItemState&cdStateSelected != 0,
				}
				paint := func(dc windows.Handle, local nativeform.Rect) {
					nativeform.DrawListHeader(dc, local, p.font, p.headerCaption(index), p.palette, state, scale)
				}
				if !nativeform.DrawBuffered(draw.HDC, bounds, paint) {
					paint(draw.HDC, bounds)
				}
				return cdResultSkipDefault
			case cdStagePostPaint:
				// Fill only an unavoidable common-control rounding remainder. Normal
				// column fitting reaches the client edge, so this does not create a
				// separate-looking strip beside the instance-count column.
				var client rect
				if ok, _, _ := pGetClientRect.Call(uintptr(p.header), uintptr(unsafe.Pointer(&client))); ok == 0 {
					return 0
				}
				left := client.Left
				count, _, _ := pSendMessage.Call(uintptr(p.header), hdmGetItemCount, 0, 0)
				if count > 0 && count != ^uintptr(0) {
					var last rect
					if ok, _, _ := pSendMessage.Call(uintptr(p.header), hdmGetItemRect, count-1, uintptr(unsafe.Pointer(&last))); ok != 0 {
						left = last.Right
					}
				}
				if left < client.Right {
					scale := int32(p.scale() + 0.5)
					if scale < 1 {
						scale = 1
					}
					bounds := nativeform.Rect{Left: left, Top: client.Top, Right: client.Right, Bottom: client.Bottom}
					nativeform.DrawListHeader(draw.HDC, bounds, p.font, "", p.palette, nativeform.ControlState{}, scale)
				}
				return 0
			}
		}
	}

	if hwnd == p.header {
		switch message {
		case wmHeaderMouseMove:
			index := p.headerItemAt(hwnd, lParam)
			if index != p.headerHover {
				p.headerHover = index
				p.invalidateHeader()
			}
			track := headerTrackMouse{Size: uint32(unsafe.Sizeof(headerTrackMouse{})), Flags: headerTMELeave, Track: hwnd}
			pHeaderTrackMouse.Call(uintptr(unsafe.Pointer(&track)))
		case wmHeaderLButtonDown:
			p.headerPressed = p.headerItemAt(hwnd, lParam)
			p.invalidateHeader()
		case wmHeaderLButtonUp:
			result, _, _ := pDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
			p.headerPressed = -1
			p.invalidateHeader()
			return result
		case wmHeaderMouseLeave:
			p.headerHover, p.headerPressed = -1, -1
			p.invalidateHeader()
		case wmHeaderCancelMode, wmHeaderCaptureLost:
			if p.headerPressed >= 0 {
				p.headerPressed = -1
				p.invalidateHeader()
			}
		case wmHeaderNCDestroy:
			p.header, p.headerHover, p.headerPressed = 0, -1, -1
		}
	}

	result, _, _ := pDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	if hwnd == p.controls[idList] {
		switch message {
		case wmHeaderMouseWheel, wmHeaderVScroll, wmHeaderKeyDown:
			p.syncListScrollbar()
		}
	}
	return result
}
