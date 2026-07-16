package processpicker

import (
	"fmt"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/automation"
	"github.com/JeffioZ/idletrigger/internal/feature/theme"
	"github.com/JeffioZ/idletrigger/internal/ui/colors"
	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
)

func (p *picker) handleCommand(id, notification uint16) {
	if notification == bnSetFocus || notification == enSetFocus {
		p.ensureControlVisible(id)
	}
	switch id {
	case idSearch:
		if notification == enChange {
			pKillTimer.Call(uintptr(p.hwnd), searchTimerID)
			pSetTimer.Call(uintptr(p.hwnd), searchTimerID, 120, 0)
		}
	case idRefresh:
		p.startLoad(processLoadManual)
	case idBrowse:
		p.browseExecutable()
	case idCancel:
		p.destroy()
	case idConfirm:
		p.confirm()
	}
}

func (p *picker) handleNotify(lParam unsafe.Pointer) {
	if lParam == nil {
		return
	}
	header := (*nmHeader)(lParam)
	if header.IDFrom != idList {
		return
	}
	switch header.Code {
	case lvnSetFocus:
		p.ensureControlVisible(idList)
	case lvnColumnClick:
		notification := (*nmListView)(lParam)
		column := int(notification.SubItem)
		if p.sortColumn == column {
			p.sortAscending = !p.sortAscending
		} else {
			p.sortColumn, p.sortAscending = column, true
		}
		p.updateHeaderLabels()
		p.applyFilter()
	case lvnItemChanged:
		notification := (*nmListView)(lParam)
		if p.populating || notification.Changed&lvifState == 0 || notification.Item < 0 {
			return
		}
		if notification.NewState&lvisStateImageMask != notification.OldState&lvisStateImageMask {
			if notification.NewState&lvisStateImageMask == 2<<12 && int(notification.Item) < len(p.visible) {
				target := p.visible[notification.Item].target
				if !canAddSelection(p.selected, target) {
					p.populating = true
					entry := lvItem{StateMask: lvisStateImageMask, State: 1 << 12}
					pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(notification.Item), uintptr(unsafe.Pointer(&entry)))
					p.populating = false
					p.setText(idStatus, fmt.Sprintf(p.text("process_picker_limit"), automation.MaxProcessesPerRule))
					return
				}
			}
			p.captureSelection()
			p.updateSelectionStatus()
		}
	case lvnGetInfoTipW:
		p.fillRowInfoTip((*nmLVGetInfoTip)(lParam))
	case nmClick:
		notification := (*nmListView)(lParam)
		if notification.Item < 0 {
			return
		}
		hit := lvHitTestInfo{Item: -1, SubItem: -1}
		hit.Point = notification.Action
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSubItemHitTest, 0, uintptr(unsafe.Pointer(&hit)))
		if hit.Item < 0 || hit.SubItem != 0 || hit.Flags&lvhtOnItemLabel == 0 || hit.Flags&lvhtOnItemStateIcon != 0 {
			return
		}
		state, _, _ := pSendMessage.Call(uintptr(p.controls[idList]), lvmGetItemState, uintptr(hit.Item), lvisStateImageMask)
		checked := uint32(2 << 12)
		if uint32(state)&lvisStateImageMask == 2<<12 {
			checked = 1 << 12
		} else if !canAddSelection(p.selected, p.visible[hit.Item].target) {
			p.setText(idStatus, fmt.Sprintf(p.text("process_picker_limit"), automation.MaxProcessesPerRule))
			return
		}
		p.populating = true
		entry := lvItem{StateMask: lvisStateImageMask, State: checked}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(hit.Item), uintptr(unsafe.Pointer(&entry)))
		p.populating = false
		p.captureSelection()
		p.updateSelectionStatus()
	}
}

func (p *picker) fillRowInfoTip(info *nmLVGetInfoTip) {
	if info == nil || info.Text == nil || info.TextMax <= 1 || info.Item < 0 || int(info.Item) >= len(p.visible) {
		return
	}
	value := p.visible[info.Item]
	if !p.listCellTruncated(value.name, 0, int(40*p.scale()+0.5)) &&
		!p.listCellTruncated(value.description, 1, int(12*p.scale()+0.5)) {
		return
	}
	label := value.name
	if value.description != "" {
		label = fmt.Sprintf(p.text("process_name_description"), value.name, value.description)
	}
	writeUTF16Text(info.Text, info.TextMax, label)
}

func (p *picker) listCellTruncated(value string, column, inset int) bool {
	if value == "" || p.controls[idList] == 0 {
		return false
	}
	text, _ := windows.UTF16PtrFromString(value)
	width, _, _ := pSendMessage.Call(uintptr(p.controls[idList]), lvmGetStringWidthW, 0, uintptr(unsafe.Pointer(text)))
	columnWidth, _, _ := pSendMessage.Call(uintptr(p.controls[idList]), lvmGetColumnWidth, uintptr(column), 0)
	return int(width)+inset > int(columnWidth)
}

func writeUTF16Text(destination *uint16, capacity int32, value string) {
	if destination == nil || capacity <= 0 {
		return
	}
	buffer := unsafe.Slice(destination, int(capacity))
	encoded := utf16.Encode([]rune(value))
	count := min(len(encoded), len(buffer)-1)
	copy(buffer[:count], encoded[:count])
	buffer[count] = 0
}

func (p *picker) text(key string) string { return p.options.Text(key) }
func (p *picker) setText(id uint16, value string) {
	if p.controls[id] == 0 {
		return
	}
	p.labels[id] = value
	text, _ := windows.UTF16PtrFromString(value)
	pSetWindowText.Call(uintptr(p.controls[id]), uintptr(unsafe.Pointer(text)))
	pInvalidateRect.Call(uintptr(p.controls[id]), 0, 0)
}
func (p *picker) show(id uint16, visible bool) {
	command := uintptr(0)
	if visible {
		command = 5
	}
	pShowWindow.Call(uintptr(p.controls[id]), command)
}
func (p *picker) enable(id uint16, enabled bool) {
	value := uintptr(0)
	if enabled {
		value = 1
	}
	pEnableWindow.Call(uintptr(p.controls[id]), value)
	pInvalidateRect.Call(uintptr(p.controls[id]), 0, 0)
}
func (p *picker) controlText(id uint16) string {
	hwnd := p.controls[id]
	length, _, _ := pSendMessage.Call(uintptr(hwnd), wmGetTextLength, 0, 0)
	buffer := make([]uint16, int(length)+1)
	if len(buffer) > 0 {
		pSendMessage.Call(uintptr(hwnd), wmGetText, uintptr(len(buffer)), uintptr(unsafe.Pointer(&buffer[0])))
	}
	return windows.UTF16ToString(buffer)
}

func (p *picker) scale() float64 {
	if p.captureScale > 0 {
		return p.captureScale
	}
	if p.dpiScale > 0 {
		return p.dpiScale
	}
	return p.windowScale()
}

func (p *picker) windowScale() float64 {
	if p.hwnd == 0 {
		return 1
	}
	dpi, _, _ := pGetDpiForWindow.Call(uintptr(p.hwnd))
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

func (p *picker) positionInWorkArea(workArea *nativeform.Rect) {
	scale := p.scale()
	anchor := p.hwnd
	if p.options.Owner != 0 {
		anchor = p.options.Owner
	}
	suggested := p.pendingSuggested
	p.pendingSuggested = nil
	_, err := nativeform.PlaceWindow(nativeform.WindowPlacement{
		Window: p.hwnd, Anchor: anchor, Owner: p.options.Owner,
		Style: p.style, ExStyle: p.exStyle,
		ClientWidth: int(windowWidth*scale + 0.5), ClientHeight: int(windowHeight*scale + 0.5),
		DPI: uint32(scale*96 + 0.5), Suggested: suggested, WorkArea: workArea,
	})
	if err != nil {
		p.layoutErr = err
		return
	}
	physicalWidth, physicalHeight, err := nativeform.ClientSize(p.hwnd)
	if err != nil {
		p.layoutErr = err
		return
	}
	p.layoutErr = nil
	p.viewportWidth = max(1, int(float64(physicalWidth)/scale))
	p.viewportHeight = max(1, int(float64(physicalHeight)/scale))
	maximum := max(0, windowHeight-p.viewportHeight)
	p.contentOffset = max(0, min(p.contentOffset, maximum))
	if p.contentScroll != nil {
		p.contentScroll.SetScale(scale)
		barWidth := max(1, int(float64(nativeform.ScrollbarWidth)*scale+0.5))
		inset := max(1, int(2*scale+0.5))
		p.contentScroll.SetBounds(physicalWidth-barWidth-inset, inset, barWidth, max(1, physicalHeight-2*inset))
		p.contentScroll.SetMetrics(windowHeight, max(1, p.viewportHeight), p.contentOffset)
	}
	p.layout()
}

func (p *picker) layout() {
	const pad, gap = nativeform.FormPadding, nativeform.ControlGap
	layoutWidth := min(windowWidth, max(1, p.viewportWidth))
	reserve := 0
	if windowHeight > p.viewportHeight {
		reserve = nativeform.ScrollbarWidth + 4
	}
	contentWidth := max(1, layoutWidth-2*pad-reserve)
	refreshWidth, browseWidth := 132, 146
	if contentWidth < 500 {
		refreshWidth, browseWidth = 96, 112
	}
	searchWidth := max(80, contentWidth-refreshWidth-browseWidth-2*gap)
	if searchWidth+refreshWidth+browseWidth+2*gap > contentWidth {
		remaining := max(2, contentWidth-searchWidth-2*gap)
		refreshWidth = remaining / 2
		browseWidth = remaining - refreshWidth
	}

	p.place(idHeading, pad, 16, contentWidth, 20)
	p.place(idHelper, pad, 40, contentWidth, 30)
	p.place(idSearchSurface, pad, 78, searchWidth, 34)
	p.place(idSearch, pad+2, 85, max(1, searchWidth-4), 20)
	p.place(idRefresh, pad+searchWidth+gap, 78, refreshWidth, 34)
	p.place(idBrowse, pad+searchWidth+gap+refreshWidth+gap, 78, browseWidth, 34)
	p.place(idListSurface, pad, 120, contentWidth, 180)
	p.place(idList, pad+2, 122, max(1, contentWidth-4), 176)
	p.place(idEmpty, pad+24, 180, max(1, contentWidth-48), 40)
	p.place(idStatus, pad, 308, contentWidth, 20)
	p.place(idPreviewTitle, pad, 336, contentWidth, 20)
	p.place(idPreviewSurface, pad, 360, contentWidth, 58)
	p.place(idPreview, pad+2, 362, max(1, contentWidth-4), 54)
	p.place(idPrivacy, pad, 426, contentWidth, 22)
	buttonGap := gap
	buttonWidth := min(106, max(1, (contentWidth-buttonGap)/2))
	p.place(idCancel, pad+contentWidth-2*buttonWidth-buttonGap, 456, buttonWidth, 36)
	p.place(idConfirm, pad+contentWidth-buttonWidth, 456, buttonWidth, 36)
	p.resizeColumns()
	p.syncListScrollbarBounds()
	p.syncListScrollbar()
	p.syncPreviewScrollbarBounds()
	if p.previewScroll != nil {
		p.previewScroll.Sync()
	}
}

func (p *picker) place(id uint16, x, y, width, height int) {
	p.bounds[id] = logicalBounds{X: x, Y: y, Width: width, Height: height}
	control := p.controls[id]
	if control == 0 {
		return
	}
	scale := p.scale()
	pSetWindowPos.Call(uintptr(control), 0,
		uintptr(int(float64(x)*scale)), uintptr(int(float64(y-p.contentOffset)*scale)),
		uintptr(max(1, int(float64(width)*scale))), uintptr(max(1, int(float64(height)*scale))),
		swpNoZOrder|swpNoActivate)
}

func (p *picker) scrollContentTo(position int) {
	maximum := max(0, windowHeight-p.viewportHeight)
	position = max(0, min(position, maximum))
	if position == p.contentOffset {
		return
	}
	p.contentOffset = position
	p.layout()
	if p.contentScroll != nil {
		p.contentScroll.SetMetrics(windowHeight, max(1, p.viewportHeight), p.contentOffset)
	}
}

func (p *picker) ensureControlVisible(id uint16) {
	bounds, ok := p.bounds[id]
	if !ok || windowHeight <= p.viewportHeight {
		return
	}
	position := p.contentOffset
	if bounds.Y < position {
		position = bounds.Y
	} else if bounds.Y+bounds.Height > position+p.viewportHeight {
		position = bounds.Y + bounds.Height - p.viewportHeight
	}
	p.scrollContentTo(position)
}

func (p *picker) scrollWheel(wParam uintptr) bool {
	if windowHeight <= p.viewportHeight {
		return false
	}
	delta := int16(wParam >> 16)
	if delta > 0 {
		p.scrollContentTo(p.contentOffset - 48)
	} else if delta < 0 {
		p.scrollContentTo(p.contentOffset + 48)
	}
	return delta != 0
}

func (p *picker) applyTheme() {
	p.themeDark = theme.Current() == theme.ModeDark
	if p.themeOverride != nil {
		p.themeDark = *p.themeOverride
	}
	p.palette = colors.ForTheme(p.themeDark)
	p.releaseBrushes()
	p.windowBrush = processBrush(p.palette.WindowBackground)
	p.surfaceBrush = processBrush(p.palette.Surface)
	nativeform.ApplyFrame(p.hwnd, p.themeDark)
	scale := p.scale()
	p.icons.Apply(p.hwnd, p.themeDark, int(32*scale+0.5), int(16*scale+0.5), false)
	for _, control := range p.controls {
		nativeform.ApplyControl(control, p.themeDark)
		pInvalidateRect.Call(uintptr(control), 0, 0)
	}
	p.applyListTheme()
	_ = p.applyStateImages()
	if p.scrollbar != nil {
		p.scrollbar.SetTheme(p.palette, p.palette.Surface)
		p.syncListScrollbar()
	}
	if p.previewScroll != nil {
		p.previewScroll.SetTheme(p.palette, p.palette.Surface)
		p.previewScroll.Sync()
	}
	if p.contentScroll != nil {
		p.contentScroll.SetTheme(p.palette, p.palette.WindowBackground)
	}
	if p.searchCue != nil {
		p.searchCue.SetTheme(p.palette.MutedText)
	}
	nativeform.ApplyTooltip(p.tooltip, p.themeDark, p.palette)
	if p.hwnd != 0 {
		pInvalidateRect.Call(uintptr(p.hwnd), 0, 0)
	}
}

func (p *picker) applyListTheme() {
	list := p.controls[idList]
	if list == 0 {
		return
	}
	pSendMessage.Call(uintptr(list), lvmSetBackgroundColor, 0, uintptr(p.palette.Surface))
	pSendMessage.Call(uintptr(list), lvmSetTextColor, 0, uintptr(p.palette.PrimaryText))
	pSendMessage.Call(uintptr(list), lvmSetTextBackground, 0, uintptr(p.palette.Surface))
	header, _, _ := pSendMessage.Call(uintptr(list), lvmGetHeader, 0, 0)
	if header != 0 {
		nativeform.ApplyControl(windows.Handle(header), p.themeDark)
		pInvalidateRect.Call(header, 0, 0)
	}
}

func processBrush(color uint32) windows.Handle {
	return createBrushForPicker(color)
}

func (p *picker) releaseBrushes() {
	for _, brush := range []windows.Handle{p.windowBrush, p.surfaceBrush} {
		if brush != 0 {
			pDeleteObject.Call(uintptr(brush))
		}
	}
	p.windowBrush, p.surfaceBrush = 0, 0
}

func (p *picker) rebuildForDPI() {
	scale := p.scale()
	newFont, _ := newFontForPicker(int32(14*scale+0.5), 400, p.options.Chinese)
	if newFont == 0 {
		return
	}
	oldFont := p.font
	p.font = newFont
	for _, control := range p.controls {
		pSendMessage.Call(uintptr(control), wmSetFont, uintptr(p.font), 1)
	}
	p.positionInWorkArea(nil)
	_ = p.applyStateImages()
	if p.scrollbar != nil {
		p.scrollbar.SetScale(scale)
		p.syncListScrollbarBounds()
		p.syncListScrollbar()
	}
	if p.previewScroll != nil {
		p.previewScroll.SetScale(scale)
		p.syncPreviewScrollbarBounds()
		p.previewScroll.Sync()
	}
	if p.searchCue != nil {
		p.searchCue.SetScale(scale)
	}
	if p.tooltip != 0 {
		pSendMessage.Call(uintptr(p.tooltip), ttmSetMaxTipWidth, 0, uintptr(int(360*scale)))
	}
	if oldFont != 0 {
		pDeleteObject.Call(uintptr(oldFont))
	}
}

func wndProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	activeMu.Lock()
	p := active
	activeMu.Unlock()
	if p == nil || p.hwnd != hwnd {
		result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		return result
	}
	switch message {
	case wmActivate:
		if uint16(wParam) != 0 && !p.captureHost && shouldAutoRefreshProcessPicker(p.lastSnapshot, time.Now(), p.lastScanDuration, p.loading) {
			p.startLoad(processLoadAutomatic)
		}
	case wmClose:
		p.destroy()
		return 0
	case wmCommand:
		p.handleCommand(uint16(wParam), uint16(wParam>>16))
		return 0
	case wmDrawItem:
		if p.drawOwnerItem((*drawItem)(nativeform.MessagePointer(lParam))) {
			return 1
		}
	case wmNotify:
		p.handleNotify(nativeform.MessagePointer(lParam))
		return 0
	case wmTimer:
		if wParam == searchTimerID {
			pKillTimer.Call(uintptr(hwnd), searchTimerID)
			p.applyFilter()
		}
		return 0
	case wmMouseWheel:
		if p.scrollWheel(wParam) {
			return 0
		}
	case wmPaint:
		nativeform.PaintWindowBackground(hwnd, p.windowBrush)
		return 0
	case wmEraseBkgnd:
		var bounds rect
		pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&bounds)))
		pFillRect.Call(wParam, uintptr(unsafe.Pointer(&bounds)), uintptr(p.windowBrush))
		return 1
	case wmCtlColorStatic:
		color := p.palette.PrimaryText
		control := windows.Handle(lParam)
		if control == p.controls[idHelper] || control == p.controls[idStatus] || control == p.controls[idPrivacy] || control == p.controls[idPreviewTitle] {
			color = p.palette.SecondaryText
		}
		pSetTextColor.Call(wParam, uintptr(color))
		backgroundColor := p.palette.WindowBackground
		brush := p.windowBrush
		if lParam == uintptr(p.controls[idEmpty]) {
			backgroundColor = p.palette.Surface
			brush = p.surfaceBrush
		}
		pSetBkColor.Call(wParam, uintptr(backgroundColor))
		pSetBkMode.Call(wParam, opaque)
		return uintptr(brush)
	case wmCtlColorEdit, wmCtlColorList:
		pSetTextColor.Call(wParam, uintptr(p.palette.PrimaryText))
		pSetBkColor.Call(wParam, uintptr(p.palette.Surface))
		return uintptr(p.surfaceBrush)
	case wmSettingChange, wmSysColorChange, wmThemeChanged:
		p.applyTheme()
		return 0
	case wmDpiChanged:
		dpi := uint32(wParam & 0xffff)
		if dpi == 0 {
			dpi = 96
		}
		p.dpiScale = float64(dpi) / 96
		if lParam != 0 {
			suggested := nativeform.Rect(*(*rect)(nativeform.MessagePointer(lParam)))
			p.pendingSuggested = &suggested
		}
		p.rebuildForDPI()
		scale := p.scale()
		p.icons.Apply(p.hwnd, p.themeDark, int(32*scale+0.5), int(16*scale+0.5), true)
		return 0
	case wmDestroy:
		p.releaseResources()
		return 0
	}
	result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}
