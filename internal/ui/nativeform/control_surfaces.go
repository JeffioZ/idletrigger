package nativeform

import (
	"fmt"

	"golang.org/x/sys/windows"
)

type ControlSurfaceOptions struct {
	ControlID, SurfaceID uint16
	Control, Surface     windows.Handle
	CueText              string
	CueColor             uint32
	Scale                float64
	Tracker              *InteractionTracker
	NewCue               func(windows.Handle, string, uint32, float64) (*CueBanner, error)
}

type ControlSurface struct {
	ControlID, SurfaceID uint16
	Control, Surface     windows.Handle
	cue                  *CueBanner
}

// ControlSurfaceSet is the single registry for a native input/list control and
// its owner-drawn form surface. The backing surface must be created with
// WS_CLIPSIBLINGS so a later surface paint cannot cover the native control.
// Optional CueBanner ownership follows the same lifecycle, so theme, DPI and
// destruction cannot be updated on only one half.
type ControlSurfaceSet struct {
	byControl map[uint16]*ControlSurface
	bySurface map[uint16]*ControlSurface
}

func (s *ControlSurfaceSet) Add(options ControlSurfaceOptions) (*ControlSurface, error) {
	if options.ControlID == 0 || options.SurfaceID == 0 || options.Control == 0 || options.Surface == 0 {
		return nil, fmt.Errorf("control surface IDs and handles are required")
	}
	if s.byControl == nil {
		s.byControl = make(map[uint16]*ControlSurface)
		s.bySurface = make(map[uint16]*ControlSurface)
	}
	if s.byControl[options.ControlID] != nil || s.bySurface[options.SurfaceID] != nil {
		return nil, fmt.Errorf("duplicate control surface binding")
	}
	field := &ControlSurface{
		ControlID: options.ControlID, SurfaceID: options.SurfaceID,
		Control: options.Control, Surface: options.Surface,
	}
	s.byControl[options.ControlID] = field
	s.bySurface[options.SurfaceID] = field
	if options.Tracker != nil {
		options.Tracker.Track(options.Control, options.Surface)
	}
	if options.CueText != "" {
		newCue := options.NewCue
		if newCue == nil {
			newCue = NewCueBanner
		}
		cue, err := newCue(options.Control, options.CueText, options.CueColor, options.Scale)
		if err != nil {
			// The native field remains fully usable when an optional visual hint
			// cannot be installed; keep the binding so layout and interaction do
			// not fall back to a second code path.
			return field, err
		}
		field.cue = cue
	}
	return field, nil
}

func (s *ControlSurfaceSet) ForControl(id uint16) (*ControlSurface, bool) {
	if s == nil {
		return nil, false
	}
	field := s.byControl[id]
	return field, field != nil
}

func (s *ControlSurfaceSet) ForSurface(id uint16) (*ControlSurface, bool) {
	if s == nil {
		return nil, false
	}
	field := s.bySurface[id]
	return field, field != nil
}

func (s *ControlSurfaceSet) SetCueTheme(color uint32) {
	if s == nil {
		return
	}
	for _, field := range s.byControl {
		if field.cue != nil {
			field.cue.SetTheme(color)
		}
	}
}

// CueColor identifies an EDIT currently displaying its cue and returns the
// current theme color for the owner's WM_CTLCOLOREDIT handler.
func (s *ControlSurfaceSet) CueColor(hwnd windows.Handle) (uint32, bool) {
	if s == nil || hwnd == 0 {
		return 0, false
	}
	for _, field := range s.byControl {
		if field.cue != nil && field.cue.edit == hwnd && field.cue.displaying {
			return field.cue.color, true
		}
	}
	return 0, false
}

// LogicalText removes temporary cue display text at the single application
// read boundary. Windows still sees the native text while painting the EDIT;
// search, validation and dirty-state code see the field as empty.
func (s *ControlSurfaceSet) LogicalText(hwnd windows.Handle, value string) string {
	if s == nil || hwnd == 0 {
		return value
	}
	for _, field := range s.byControl {
		if field.cue != nil && field.cue.edit == hwnd && field.cue.displaying {
			return ""
		}
	}
	return value
}

// PrepareCues ensures empty native fields have their display hint before a
// complete first frame or rebuilt editor view is presented.
func (s *ControlSurfaceSet) PrepareCues() {
	if s == nil {
		return
	}
	for _, field := range s.byControl {
		if field.cue != nil {
			field.cue.ensureVisible()
			field.cue.invalidate()
		}
	}
}

func (s *ControlSurfaceSet) SetScale(scale float64) {
	if s == nil {
		return
	}
	for _, field := range s.byControl {
		if field.cue != nil {
			field.cue.SetScale(scale)
		}
	}
}

func (s *ControlSurfaceSet) Close() {
	if s == nil {
		return
	}
	for _, field := range s.byControl {
		if field.cue != nil {
			field.cue.Close()
			field.cue = nil
		}
	}
	s.byControl = nil
	s.bySurface = nil
}
