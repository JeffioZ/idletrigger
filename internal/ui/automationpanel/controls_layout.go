package automationpanel

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/feature/theme"
	"github.com/JeffioZ/idletrigger/internal/ui/colors"
	"github.com/JeffioZ/idletrigger/internal/ui/font"
	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
)

func (p *panel) namedLabel(id uint16, value string, useFont windows.Handle) {
	p.child("STATIC", value, wsChild|ssLeft, 0, 0, 1, 1, id, useFont)
}
func (p *panel) edit(id uint16, value string) {
	surfaceID := idFieldSurfaceBase + id
	p.child("STATIC", "", wsChild|ssOwnerDraw, 0, 0, 1, 1, surfaceID, p.font)
	hwnd := p.child("EDIT", value, wsChild|wsTabStop|esAutoHScroll, 0, 0, 1, 1, id, p.font)
	p.fieldSurfaces[id] = surfaceID
	p.surfaceFields[surfaceID] = id
	p.interaction.Track(hwnd, p.controls[surfaceID])
	margin := int(6*p.scale() + 0.5)
	pSendMessage.Call(uintptr(hwnd), emSetMargins, 3, uintptr(margin|(margin<<16)))
}
func (p *panel) combo(id uint16, x, y, width int, labels []string) {
	p.child("BUTTON", "", wsChild|wsTabStop|bsOwnerDraw, x, y, width, nativeform.FieldHeight, id, p.font)
	p.setChoiceOptions(id, labels)
}
func (p *panel) place(id uint16, x, y, width, height int, visible bool) {
	hwnd := p.controls[id]
	if hwnd == 0 {
		return
	}
	p.bounds[id] = logicalBounds{X: x, Y: y, Width: width, Height: height}
	if surfaceID, ok := p.fieldSurfaces[id]; ok {
		p.positionControl(p.controls[surfaceID], x, y, width, height)
		innerHeight := min(20, height-4)
		p.positionControl(hwnd, x+2, y+(height-innerHeight)/2, width-4, innerHeight)
		p.show(surfaceID, visible)
		p.show(id, visible)
		if visible {
			pSetWindowPos.Call(uintptr(hwnd), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		}
		return
	}
	p.positionControl(hwnd, x, y, width, height)
	p.show(id, visible)
}
func (p *panel) positionControl(hwnd windows.Handle, x, y, width, height int) {
	if hwnd == 0 {
		return
	}
	scale := p.scale()
	pSetWindowPos.Call(uintptr(hwnd), 0,
		uintptr(int(float64(x)*scale)), uintptr(int(float64(y-p.contentOffset)*scale)),
		uintptr(int(float64(width)*scale)), uintptr(int(float64(height)*scale)),
		swpNoZOrder|swpNoActivate)
}
func (p *panel) placeCombo(id uint16, x, y, width int, visible bool) {
	p.place(id, x, y, width, nativeform.FieldHeight, visible)
}
func (p *panel) child(className, value string, style uintptr, x, y, width, height int, id uint16, useFont windows.Handle) windows.Handle {
	class, _ := windows.UTF16PtrFromString(className)
	caption, _ := windows.UTF16PtrFromString(value)
	scale := p.scale()
	hwnd, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(caption)), style, uintptr(int(float64(x)*scale)), uintptr(int(float64(y)*scale)), uintptr(int(float64(width)*scale)), uintptr(int(float64(height)*scale)), uintptr(p.hwnd), uintptr(id), 0, 0)
	if hwnd != 0 && useFont != 0 {
		pSendMessage.Call(hwnd, wmSetFont, uintptr(useFont), 1)
	}
	if hwnd != 0 {
		nativeform.ApplyControl(windows.Handle(hwnd), p.themeDark)
	}
	if id != 0 {
		p.controls[id] = windows.Handle(hwnd)
		p.labels[id] = value
		p.bounds[id] = logicalBounds{X: x, Y: y, Width: width, Height: height}
		if strings.EqualFold(className, "BUTTON") && style&bsOwnerDraw != 0 {
			p.interaction.Track(windows.Handle(hwnd), windows.Handle(hwnd))
		}
	} else if hwnd != 0 {
		p.anonymous = append(p.anonymous, windows.Handle(hwnd))
	}
	return windows.Handle(hwnd)
}
func (p *panel) addTooltip(id uint16, key string) {
	p.addTooltipValue(id, p.t(key))
}

func (p *panel) addTooltipValue(id uint16, value string) {
	control := p.controls[id]
	if control == 0 {
		return
	}
	if p.tooltip == 0 {
		class, _ := windows.UTF16PtrFromString("tooltips_class32")
		tip, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(class)), 0, wsPopup|ttsAlwaysTip|ttsNoPrefix, 0, 0, 0, 0, uintptr(p.hwnd), 0, 0, 0)
		if tip == 0 {
			return
		}
		p.tooltip = windows.Handle(tip)
		pSendMessage.Call(tip, ttmSetMaxTipWidth, 0, uintptr(int(360*p.scale())))
		nativeform.ApplyTooltip(p.tooltip, p.themeDark, p.palette)
	}
	info := toolInfo{Size: uint32(unsafe.Sizeof(toolInfo{})), Flags: ttfIDIsHwnd | ttfSubclass, Hwnd: p.hwnd, ID: uintptr(control)}
	pSendMessage.Call(uintptr(p.tooltip), ttmDelTool, 0, uintptr(unsafe.Pointer(&info)))
	text, err := windows.UTF16FromString(value)
	if err != nil || len(text) == 0 {
		return
	}
	if p.tooltipText == nil {
		p.tooltipText = make(map[uint16][]uint16)
	}
	p.tooltipText[id] = text
	info.Text = &text[0]
	pSendMessage.Call(uintptr(p.tooltip), ttmAddTool, 0, uintptr(unsafe.Pointer(&info)))
}
func (p *panel) setText(id uint16, value string) {
	if p.controls[id] == 0 {
		return
	}
	text, _ := windows.UTF16PtrFromString(value)
	pSetWindowText.Call(uintptr(p.controls[id]), uintptr(unsafe.Pointer(text)))
	p.labels[id] = value
	pInvalidateRect.Call(uintptr(p.controls[id]), 0, 0)
}
func (p *panel) setCaption(value string) {
	text, _ := windows.UTF16PtrFromString(value)
	pSetWindowText.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(text)))
}
func (p *panel) show(id uint16, visible bool) {
	command := uintptr(0)
	if visible {
		command = 5
	}
	pShowWindow.Call(uintptr(p.controls[id]), command)
	if surfaceID, ok := p.fieldSurfaces[id]; ok {
		pShowWindow.Call(uintptr(p.controls[surfaceID]), command)
	}
}

func (p *panel) hideControls(ids []uint16) {
	for _, id := range ids {
		p.show(id, false)
	}
}

func (p *panel) showControls(ids []uint16) {
	for _, id := range ids {
		p.show(id, true)
	}
}

func (p *panel) controlText(id uint16) string {
	hwnd := p.controls[id]
	length, _, _ := pSendMessage.Call(uintptr(hwnd), wmGetTextLength, 0, 0)
	buf := make([]uint16, int(length)+1)
	if len(buf) > 0 {
		pSendMessage.Call(uintptr(hwnd), wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	}
	return windows.UTF16ToString(buf)
}
func (p *panel) selectCombo(id uint16, index int) {
	choice := p.choices[id]
	if choice == nil || len(choice.labels) == 0 {
		return
	}
	if index < 0 || index >= len(choice.labels) {
		index = 0
	}
	choice.selected = index
	p.setText(id, choice.labels[index])
}
func (p *panel) comboIndex(id uint16) int {
	choice := p.choices[id]
	if choice == nil || choice.selected < 0 || choice.selected >= len(choice.labels) {
		return 0
	}
	return choice.selected
}
func (p *panel) setChecked(id uint16, value bool) {
	p.checks[id] = value
	if p.controls[id] != 0 {
		pInvalidateRect.Call(uintptr(p.controls[id]), 0, 0)
	}
}
func (p *panel) checked(id uint16) bool {
	return p.checks[id]
}
func (p *panel) enable(id uint16, value bool) {
	flag := uintptr(0)
	if value {
		flag = 1
	}
	pEnableWindow.Call(uintptr(p.controls[id]), flag)
	p.invalidateControl(id)
	if surfaceID, ok := p.fieldSurfaces[id]; ok {
		pInvalidateRect.Call(uintptr(p.controls[surfaceID]), 0, 0)
	}
}
func (p *panel) t(key string) string { return p.text(key) }
func (p *panel) scale() float64 {
	if p.captureScale > 0 {
		return p.captureScale
	}
	if p.dpiScale > 0 {
		return p.dpiScale
	}
	return p.windowScale()
}
func (p *panel) windowScale() float64 {
	if p.hwnd == 0 {
		return 1
	}
	dpi, _, _ := pGetDpiForWindow.Call(uintptr(p.hwnd))
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}
func (p *panel) resize(width, height int) {
	p.resizeInWorkArea(width, height, p.layoutWorkArea)
}

func (p *panel) resizeInWorkArea(width, height int, workArea *nativeform.Rect) {
	previousViewportWidth := p.viewportWidth
	previousWorkArea := p.layoutWorkArea
	p.layoutWorkArea = workArea
	defer func() { p.layoutWorkArea = previousWorkArea }()
	p.clientWidth, p.clientHeight = width, height
	scale := p.scale()
	anchor := p.hwnd
	if p.state.Owner != 0 {
		anchor = p.state.Owner
	}
	suggested := p.pendingSuggested
	p.pendingSuggested = nil
	_, err := nativeform.PlaceWindow(nativeform.WindowPlacement{
		Window: p.hwnd, Anchor: anchor, Owner: p.state.Owner,
		Style: p.style, ExStyle: p.exStyle,
		ClientWidth: int(float64(width)*scale + 0.5), ClientHeight: int(float64(height)*scale + 0.5),
		DPI: uint32(scale*96 + 0.5), Suggested: suggested, WorkArea: workArea,
	})
	if err != nil {
		p.layoutErr = err
		return
	}
	p.layoutErr = nil
	p.syncContentViewport()
	if p.viewportWidth != previousViewportWidth {
		if p.view == editorView && p.editorReady && !p.layoutingEditor {
			p.layoutEditor()
		} else if p.view == managerView && p.managerReady {
			p.layoutManager()
		}
	}
}

func (p *panel) moveToCursorMonitor() {
	var cursor point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	pSetWindowPos.Call(uintptr(p.hwnd), 0, uintptr(cursor.X), uintptr(cursor.Y), 1, 1, 0x0010)
}

func (p *panel) syncContentViewport() {
	if p.hwnd == 0 {
		return
	}
	physicalWidth, physicalHeight, err := nativeform.ClientSize(p.hwnd)
	if err != nil {
		p.layoutErr = err
		return
	}
	scale := p.scale()
	p.viewportWidth = max(1, int(float64(physicalWidth)/scale))
	p.viewportHeight = max(1, int(float64(physicalHeight)/scale))
	maximum := max(0, p.clientHeight-p.viewportHeight)
	p.contentOffset = max(0, min(p.contentOffset, maximum))
	if p.contentScroll != nil {
		p.contentScroll.SetScale(scale)
		barWidth := max(1, int(float64(nativeform.ScrollbarWidth)*scale+0.5))
		inset := max(1, int(2*scale+0.5))
		p.contentScroll.SetBounds(physicalWidth-barWidth-inset, inset, barWidth, max(1, physicalHeight-2*inset))
		p.contentScroll.SetMetrics(max(1, p.clientHeight), max(1, p.viewportHeight), p.contentOffset)
	}
	p.repositionContent()
}

func (p *panel) scrollContentTo(position int) {
	maximum := max(0, p.clientHeight-p.viewportHeight)
	position = max(0, min(position, maximum))
	if position == p.contentOffset {
		return
	}
	p.closeChoice(false)
	p.contentOffset = position
	p.repositionContent()
	if p.contentScroll != nil {
		p.contentScroll.SetMetrics(max(1, p.clientHeight), max(1, p.viewportHeight), p.contentOffset)
	}
}

func (p *panel) repositionContent() {
	for id, bounds := range p.bounds {
		if _, fieldSurface := p.surfaceFields[id]; fieldSurface {
			continue
		}
		hwnd := p.controls[id]
		if hwnd == 0 {
			continue
		}
		if surfaceID, field := p.fieldSurfaces[id]; field {
			p.positionControl(p.controls[surfaceID], bounds.X, bounds.Y, bounds.Width, bounds.Height)
			innerHeight := min(20, bounds.Height-4)
			p.positionControl(hwnd, bounds.X+2, bounds.Y+(bounds.Height-innerHeight)/2, bounds.Width-4, innerHeight)
			continue
		}
		p.positionControl(hwnd, bounds.X, bounds.Y, bounds.Width, bounds.Height)
	}
	p.syncManagerScrollbarBounds()
}

func (p *panel) ensureControlVisible(id uint16) {
	bounds, ok := p.bounds[id]
	if !ok || p.clientHeight <= p.viewportHeight {
		return
	}
	top, bottom := bounds.Y, bounds.Y+bounds.Height
	position := p.contentOffset
	if top < position {
		position = top
	} else if bottom > position+p.viewportHeight {
		position = bottom - p.viewportHeight
	}
	p.scrollContentTo(position)
}

func (p *panel) scrollWheel(wParam uintptr) bool {
	if p.clientHeight <= p.viewportHeight {
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

func (p *panel) rebuildForDPI() {
	view, editing, draft, contentOffset := p.view, p.editing, p.draft, p.contentOffset
	if view == editorView {
		p.syncDraft()
		draft = p.draft
	}
	p.clearControls()
	if p.font != 0 {
		pDeleteObject.Call(uintptr(p.font))
	}
	if p.sectionFont != 0 {
		pDeleteObject.Call(uintptr(p.sectionFont))
	}
	scale := p.scale()
	p.font, _ = font.New(int32(14*scale+0.5), 400, p.state.Chinese)
	p.sectionFont, _ = font.New(int32(14*scale+0.5), 600, p.state.Chinese)
	if view == editorView {
		p.showEditorDraft(editing, draft)
	} else {
		p.showManager()
	}
	p.scrollContentTo(contentOffset)
}
func (p *panel) applyTheme() {
	p.themeDark = theme.Current() == theme.ModeDark
	if p.themeOverride != nil {
		p.themeDark = *p.themeOverride
	}
	p.palette = colors.ForTheme(p.themeDark)
	p.releaseBrushes()
	p.windowBrush = makeBrush(p.palette.WindowBackground)
	p.surfaceBrush = makeBrush(p.palette.Surface)
	p.disabledBrush = makeBrush(p.palette.DisabledSurface)
	nativeform.ApplyFrame(p.hwnd, p.themeDark)
	scale := p.scale()
	p.icons.Apply(p.hwnd, p.themeDark, int(32*scale+0.5), int(16*scale+0.5), false)
	for _, control := range p.controls {
		nativeform.ApplyControl(control, p.themeDark)
		pInvalidateRect.Call(uintptr(control), 0, 0)
	}
	for _, control := range p.anonymous {
		nativeform.ApplyControl(control, p.themeDark)
		pInvalidateRect.Call(uintptr(control), 0, 0)
	}
	nativeform.ApplyTooltip(p.tooltip, p.themeDark, p.palette)
	if p.managerScroll != nil {
		p.managerScroll.SetTheme(p.palette, p.palette.Surface)
		p.managerScroll.Sync()
	}
	if p.contentScroll != nil {
		p.contentScroll.SetTheme(p.palette, p.palette.WindowBackground)
	}
	if p.nameCue != nil {
		p.nameCue.SetTheme(p.palette.MutedText)
	}
	if p.hwnd != 0 {
		pInvalidateRect.Call(uintptr(p.hwnd), 0, 0)
	}
}

func (p *panel) drawOwnerItem(value *drawItem) bool {
	return p.drawStyledOwnerItem(value)
}

func makeBrush(color uint32) windows.Handle {
	brush, _, _ := pCreateBrush.Call(uintptr(color))
	return windows.Handle(brush)
}

func (p *panel) releaseBrushes() {
	for _, brush := range []windows.Handle{p.windowBrush, p.surfaceBrush, p.disabledBrush} {
		if brush != 0 {
			pDeleteObject.Call(uintptr(brush))
		}
	}
	p.windowBrush, p.surfaceBrush, p.disabledBrush = 0, 0, 0
}

func weekdayButtonProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr, subclassID, refData uintptr) uintptr {
	if message == wmKeyDown && (wParam == vkLeft || wParam == vkRight || wParam == vkHome || wParam == vkEnd) {
		activeMu.Lock()
		p := active
		activeMu.Unlock()
		if p != nil && p.view == editorView {
			id := p.controlID(hwnd)
			if id >= idWeekdayBase && id < idWeekdayBase+uint16(len(editorWeekdays)) {
				index := int(id - idWeekdayBase)
				switch wParam {
				case vkLeft:
					index = (index + len(editorWeekdays) - 1) % len(editorWeekdays)
				case vkRight:
					index = (index + 1) % len(editorWeekdays)
				case vkHome:
					index = 0
				case vkEnd:
					index = len(editorWeekdays) - 1
				}
				pSetFocus.Call(uintptr(p.controls[idWeekdayBase+uint16(index)]))
				return 0
			}
		}
	}
	result, _, _ := pDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}

func (p *panel) confirm(title, body string) bool {
	t, _ := windows.UTF16PtrFromString(title)
	b, _ := windows.UTF16PtrFromString(body)
	const mbYesNoWarningDefaultNo = 0x00000004 | 0x00000030 | 0x00000100
	result, _, _ := user32.NewProc("MessageBoxW").Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), mbYesNoWarningDefaultNo)
	return result == 6
}
