package processpicker

import (
	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
)

func (p *picker) drawOwnerItem(value *drawItem) bool {
	if value == nil || value.HDC == 0 {
		return false
	}
	bounds := nativeform.Rect{Left: value.Rect.Left, Top: value.Rect.Top, Right: value.Rect.Right, Bottom: value.Rect.Bottom}
	painted := nativeform.DrawBuffered(value.HDC, bounds, func(dc windows.Handle, local nativeform.Rect) {
		buffered := *value
		buffered.HDC = dc
		buffered.Rect = rect{Left: local.Left, Top: local.Top, Right: local.Right, Bottom: local.Bottom}
		p.drawOwnerItemDirect(&buffered)
	})
	if painted {
		return true
	}
	return p.drawOwnerItemDirect(value)
}

func (p *picker) drawOwnerItemDirect(value *drawItem) bool {
	if value == nil || value.HDC == 0 {
		return false
	}
	id := uint16(value.CtlID)
	bounds := nativeform.Rect{Left: value.Rect.Left, Top: value.Rect.Top, Right: value.Rect.Right, Bottom: value.Rect.Bottom}
	radius := int32(6*p.scale() + 0.5)
	if radius < 3 {
		radius = 3
	}
	if field, ok := p.surfaces.ForSurface(id); ok {
		interaction := p.interaction.State(field.Control)
		state := nativeform.ControlState{Hovered: interaction.Hovered, Focused: interaction.Focused}
		nativeform.DrawField(value.HDC, bounds, p.palette, p.palette.WindowBackground, state, radius)
		return true
	}
	control := p.controls[id]
	if control == 0 {
		return false
	}
	interaction := p.interaction.State(control)
	state := nativeform.ControlState{
		Hovered:  interaction.Hovered,
		Pressed:  interaction.Pressed || value.ItemState&odsSelected != 0,
		Focused:  interaction.FocusVisible,
		Disabled: value.ItemState&odsDisabled != 0 || !p.controlEnabled(id),
	}
	nativeform.DrawButton(value.HDC, bounds, p.font, p.labels[id], p.palette, p.palette.WindowBackground, state, radius, false)
	return true
}

func (p *picker) controlEnabled(id uint16) bool {
	control := p.controls[id]
	if control == 0 {
		return false
	}
	enabled, _, _ := pIsWindowEnabled.Call(uintptr(control))
	return enabled != 0
}
