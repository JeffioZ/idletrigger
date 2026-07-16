package nativeform

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/ui/colors"
)

type ListboxScrollbarOptions struct {
	Parent     windows.Handle
	Listbox    windows.Handle
	Palette    colors.Palette
	Background uint32
	Scale      float64
}

// ListboxScrollbar connects the shared themed scrollbar to a native listbox.
// The listbox keeps its keyboard and wheel behavior while the unreliable
// native non-client scrollbar remains hidden.
type ListboxScrollbar struct {
	listbox   windows.Handle
	scrollbar *Scrollbar
	active    bool
	closed    bool
}

type listboxClientRect struct{ Left, Top, Right, Bottom int32 }

const (
	listboxSubclassID = 0x49544C42
	lbWMNCDestroy     = 0x0082
	lbWMKeyDown       = 0x0100
	lbWMVScroll       = 0x0115
	lbWMMouseWheel    = 0x020A
	lbGetCount        = 0x018B
	lbGetTopIndex     = 0x018E
	lbSetTopIndex     = 0x0197
	lbGetItemRect     = 0x0198
	lbGetItemHeight   = 0x01A1
	lbSBVert          = 1
)

var (
	listboxUser32          = windows.NewLazySystemDLL("user32.dll")
	listboxComctl32        = windows.NewLazySystemDLL("comctl32.dll")
	lbSendMessage          = listboxUser32.NewProc("SendMessageW")
	lbGetClientRect        = listboxUser32.NewProc("GetClientRect")
	lbShowScrollBar        = listboxUser32.NewProc("ShowScrollBar")
	lbSetWindowSubclass    = listboxComctl32.NewProc("SetWindowSubclass")
	lbRemoveWindowSubclass = listboxComctl32.NewProc("RemoveWindowSubclass")
	lbDefSubclassProc      = listboxComctl32.NewProc("DefSubclassProc")
	listboxCallback        = windows.NewCallback(listboxScrollbarProc)
	listboxMu              sync.Mutex
	listboxScrollbars      = make(map[windows.Handle]*ListboxScrollbar)
)

func NewListboxScrollbar(options ListboxScrollbarOptions) (*ListboxScrollbar, error) {
	if options.Parent == 0 || options.Listbox == 0 {
		return nil, fmt.Errorf("listbox scrollbar parent and listbox are required")
	}
	result := &ListboxScrollbar{listbox: options.Listbox, active: true}
	bar, err := NewScrollbar(ScrollbarOptions{
		Parent: options.Parent, Palette: options.Palette, Background: options.Background, Scale: options.Scale,
		OnChange: func(position int) {
			if result.closed || result.listbox == 0 {
				return
			}
			lbSendMessage.Call(uintptr(result.listbox), lbSetTopIndex, uintptr(position), 0)
			result.Sync()
		},
	})
	if err != nil {
		return nil, err
	}
	result.scrollbar = bar
	installed, _, callErr := lbSetWindowSubclass.Call(uintptr(options.Listbox), listboxCallback, listboxSubclassID, 0)
	if installed == 0 {
		bar.Close()
		return nil, fmt.Errorf("subclass listbox scrollbar: %w", callErr)
	}
	listboxMu.Lock()
	listboxScrollbars[options.Listbox] = result
	listboxMu.Unlock()
	lbShowScrollBar.Call(uintptr(options.Listbox), lbSBVert, 0)
	result.Sync()
	return result, nil
}

func (s *ListboxScrollbar) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	if s.listbox != 0 {
		lbRemoveWindowSubclass.Call(uintptr(s.listbox), listboxCallback, listboxSubclassID)
		listboxMu.Lock()
		delete(listboxScrollbars, s.listbox)
		listboxMu.Unlock()
		s.listbox = 0
	}
	if s.scrollbar != nil {
		s.scrollbar.Close()
		s.scrollbar = nil
	}
}

func (s *ListboxScrollbar) SetBounds(x, y, width, height int) {
	if s != nil && s.scrollbar != nil {
		s.scrollbar.SetBounds(x, y, width, height)
	}
}

func (s *ListboxScrollbar) SetTheme(palette colors.Palette, background uint32) {
	if s != nil && s.scrollbar != nil {
		s.scrollbar.SetTheme(palette, background)
	}
}

func (s *ListboxScrollbar) SetScale(scale float64) {
	if s != nil && s.scrollbar != nil {
		s.scrollbar.SetScale(scale)
	}
}

func (s *ListboxScrollbar) SetActive(active bool) {
	if s == nil || s.closed {
		return
	}
	s.active = active
	s.Sync()
}

func (s *ListboxScrollbar) Sync() {
	if s == nil || s.closed || s.scrollbar == nil || s.listbox == 0 {
		return
	}
	lbShowScrollBar.Call(uintptr(s.listbox), lbSBVert, 0)
	if !s.active {
		s.scrollbar.SetMetrics(0, 0, 0)
		return
	}
	total, _, _ := lbSendMessage.Call(uintptr(s.listbox), lbGetCount, 0, 0)
	if total == ^uintptr(0) || total == 0 {
		s.scrollbar.SetMetrics(0, 0, 0)
		return
	}
	itemHeight, _, _ := lbSendMessage.Call(uintptr(s.listbox), lbGetItemHeight, 0, 0)
	var first listboxClientRect
	if ok, _, _ := lbSendMessage.Call(uintptr(s.listbox), lbGetItemRect, 0, uintptr(unsafe.Pointer(&first))); ok != ^uintptr(0) && first.Bottom > first.Top {
		itemHeight = uintptr(first.Bottom - first.Top)
	}
	var client listboxClientRect
	lbGetClientRect.Call(uintptr(s.listbox), uintptr(unsafe.Pointer(&client)))
	page := 1
	if itemHeight > 0 {
		page = max(1, int(client.Bottom-client.Top)/int(itemHeight))
	}
	top, _, _ := lbSendMessage.Call(uintptr(s.listbox), lbGetTopIndex, 0, 0)
	s.scrollbar.SetMetrics(int(total), page, int(top))
}

func listboxScrollbarProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr, subclassID, refData uintptr) uintptr {
	listboxMu.Lock()
	bar := listboxScrollbars[hwnd]
	listboxMu.Unlock()
	result, _, _ := lbDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	if bar == nil {
		return result
	}
	switch message {
	case lbWMKeyDown, lbWMVScroll, lbWMMouseWheel:
		bar.Sync()
	case lbWMNCDestroy:
		listboxMu.Lock()
		delete(listboxScrollbars, hwnd)
		listboxMu.Unlock()
		bar.listbox = 0
		bar.closed = true
		if bar.scrollbar != nil {
			bar.scrollbar.Close()
			bar.scrollbar = nil
		}
	}
	return result
}
