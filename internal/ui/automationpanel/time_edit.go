package automationpanel

import (
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	timeEditSubclassID = 0x4954544D
	timeWMChar         = 0x0102
	timeWMKillFocus    = 0x0008
	timeWMPaste        = 0x0302
	timeEMGetSel       = 0x00B0
	timeEMSetSel       = 0x00B1
)

var timeEditCallback = windows.NewCallback(timeEditProc)

func (p *panel) timeEdit(id uint16, value string) {
	p.edit(id, value)
	if control := p.controls[id]; control != 0 {
		pSetWindowSubclass.Call(uintptr(control), timeEditCallback, timeEditSubclassID, 0)
	}
}

// normalizeTimeEditText keeps a time field in a small, predictable editing
// grammar: up to two hour digits, one separator and up to two minute digits.
// A third digit inserts the separator, while blur pads a one-digit hour.
func normalizeTimeEditText(value string, caret int, final bool) (string, int) {
	out := make([]rune, 0, 5)
	hourDigits := 0
	minuteDigits := 0
	separator := false
	inputOffset := 0
	normalizedCaret := 0

	for _, valueRune := range value {
		runeUnits := utf16.RuneLen(valueRune)
		if runeUnits < 1 {
			runeUnits = 1
		}
		mapped, allowed := normalizeTimeEditRune(valueRune)
		if allowed {
			switch {
			case mapped >= '0' && mapped <= '9' && !separator && hourDigits < 2:
				out = append(out, mapped)
				hourDigits++
			case mapped >= '0' && mapped <= '9' && !separator && minuteDigits < 2:
				out = append(out, ':', mapped)
				separator = true
				minuteDigits++
			case mapped >= '0' && mapped <= '9' && minuteDigits < 2:
				out = append(out, mapped)
				minuteDigits++
			case mapped == ':' && !separator && hourDigits > 0:
				out = append(out, mapped)
				separator = true
			}
		}
		inputOffset += runeUnits
		if inputOffset <= caret {
			normalizedCaret = len(out)
		}
	}

	if final && separator && hourDigits == 1 {
		out = append([]rune{'0'}, out...)
		if caret > 0 {
			normalizedCaret++
		}
	}
	if normalizedCaret > len(out) {
		normalizedCaret = len(out)
	}
	return string(out), normalizedCaret
}

func normalizeTimeEditRune(value rune) (rune, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value, true
	case value >= '\uFF10' && value <= '\uFF19':
		return '0' + value - '\uFF10', true
	case value == ':' || value == '\uFF1A':
		return ':', true
	default:
		return 0, false
	}
}

func timeEditProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr, subclassID, refData uintptr) uintptr {
	switch message {
	case timeWMChar:
		if wParam < 0x20 {
			break
		}
		if _, allowed := normalizeTimeEditRune(rune(wParam)); !allowed {
			return 0
		}
		result, _, _ := pDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		timeEditNormalizeWindow(hwnd, false)
		return result
	case timeWMPaste:
		result, _, _ := pDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		timeEditNormalizeWindow(hwnd, false)
		return result
	case timeWMKillFocus:
		timeEditNormalizeWindow(hwnd, true)
	}
	result, _, _ := pDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}

func timeEditNormalizeWindow(hwnd windows.Handle, final bool) {
	value := timeEditWindowText(hwnd)
	start, _ := timeEditSelection(hwnd)
	normalized, caret := normalizeTimeEditText(value, start, final)
	if normalized == value {
		return
	}
	timeEditSetTextAndSelection(hwnd, normalized, caret)
}

func timeEditWindowText(hwnd windows.Handle) string {
	length, _, _ := pSendMessage.Call(uintptr(hwnd), wmGetTextLength, 0, 0)
	buffer := make([]uint16, int(length)+1)
	if len(buffer) > 0 {
		pSendMessage.Call(uintptr(hwnd), wmGetText, uintptr(len(buffer)), uintptr(unsafe.Pointer(&buffer[0])))
	}
	return windows.UTF16ToString(buffer)
}

func timeEditSelection(hwnd windows.Handle) (int, int) {
	var start, end uint32
	pSendMessage.Call(
		uintptr(hwnd), timeEMGetSel,
		uintptr(unsafe.Pointer(&start)), uintptr(unsafe.Pointer(&end)),
	)
	return int(start), int(end)
}

func timeEditSetTextAndSelection(hwnd windows.Handle, value string, caret int) {
	text, _ := windows.UTF16PtrFromString(value)
	pSetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(text)))
	pSendMessage.Call(uintptr(hwnd), timeEMSetSel, uintptr(caret), uintptr(caret))
}
