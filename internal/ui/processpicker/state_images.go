package processpicker

import (
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
)

func (p *picker) applyStateImages() {
	list := p.controls[idList]
	if list == 0 || p.hwnd == 0 {
		return
	}
	scale := p.scale()
	if scale <= 0 {
		scale = 1
	}
	size := int32(22*scale + 0.5)
	images, _, _ := pImageListCreate.Call(uintptr(size), uintptr(size), ilcColor32, 2, 0)
	if images == 0 {
		return
	}
	if err := p.addStateImage(windows.Handle(images), size, scale, false); err != nil {
		pImageListDestroy.Call(images)
		return
	}
	if err := p.addStateImage(windows.Handle(images), size, scale, true); err != nil {
		pImageListDestroy.Call(images)
		return
	}
	previous := p.stateImages
	pSendMessage.Call(uintptr(list), lvmSetImageList, lvsilState, images)
	p.stateImages = windows.Handle(images)
	if previous != 0 {
		pImageListDestroy.Call(uintptr(previous))
	}
}

func (p *picker) addStateImage(images windows.Handle, size int32, scale float64, checked bool) error {
	dc, _, _ := pGetDC.Call(uintptr(p.hwnd))
	if dc == 0 {
		return fmt.Errorf("get process picker DC")
	}
	defer pReleaseDC.Call(uintptr(p.hwnd), dc)
	memoryDC, _, _ := pCreateCompatibleDC.Call(dc)
	if memoryDC == 0 {
		return fmt.Errorf("create checkbox DC")
	}
	defer pDeleteDC.Call(memoryDC)
	bitmap, _, _ := pCreateBitmap.Call(dc, uintptr(size), uintptr(size))
	if bitmap == 0 {
		return fmt.Errorf("create checkbox bitmap")
	}
	defer pDeleteObject.Call(bitmap)
	old, _, _ := pSelectObject.Call(memoryDC, bitmap)
	nativeform.DrawCheckboxGlyph(windows.Handle(memoryDC), nativeform.Rect{Right: size, Bottom: size}, p.palette, p.palette.Surface, nativeform.ControlState{Active: checked}, scale)
	pSelectObject.Call(memoryDC, old)
	index, _, _ := pImageListAdd.Call(uintptr(images), bitmap, 0)
	if int32(index) < 0 {
		return fmt.Errorf("add checkbox image")
	}
	return nil
}

func (p *picker) releaseStateImages() {
	if p.stateImages == 0 {
		return
	}
	if list := p.controls[idList]; list != 0 {
		pSendMessage.Call(uintptr(list), lvmSetImageList, lvsilState, 0)
	}
	pImageListDestroy.Call(uintptr(p.stateImages))
	p.stateImages = 0
}
