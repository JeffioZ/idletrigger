package nativeform

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	firstFrameTransitionsForcedDisabled = 3
	firstFrameCloak                     = 13
	firstFrameShow                      = 5
)

type firstFrameAPI struct {
	setDWMBoolean func(windows.Handle, uint32, bool) bool
	showWindow    func(windows.Handle)
	present       func(windows.Handle, ...windows.Handle)
}

var defaultFirstFrameAPI = firstFrameAPI{
	setDWMBoolean: setDWMBooleanAttribute,
	showWindow: func(window windows.Handle) {
		firstFrameShowWindow.Call(uintptr(window), firstFrameShow)
	},
	present: PresentFrame,
}

var firstFrameShowWindow = user32.NewProc("ShowWindow")

// FirstFrameOptions describes the one atomic first presentation of a native
// top-level form. RepeatShow is reserved for deterministic devtools capture
// hosts whose STARTUPINFO can override the process's first ShowWindow call.
type FirstFrameOptions struct {
	RepeatShow bool
	Controls   []windows.Handle
}

// FirstFrameGate keeps a newly created top-level HWND invisible at the DWM
// compositor until its controls, layout and first synchronous paint are
// complete. It is intentionally single-use.
type FirstFrameGate struct {
	window              windows.Handle
	cloaked             bool
	transitionsDisabled bool
	revealed            bool
	api                 firstFrameAPI
}

// FrameTransition keeps an already-visible top-level HWND out of the DWM
// presentation stream while one synchronous layout transition is committed.
// It does not change WS_VISIBLE, activation or focus.
type FrameTransition struct {
	window              windows.Handle
	cloaked             bool
	transitionsDisabled bool
	committed           bool
	api                 firstFrameAPI
}

// BeginFirstFrame prepares a hidden top-level window for atomic presentation.
// Unsupported DWM attributes safely retain the normal hidden-window path.
func BeginFirstFrame(window windows.Handle) FirstFrameGate {
	return beginFirstFrameWith(window, defaultFirstFrameAPI)
}

func beginFirstFrameWith(window windows.Handle, api firstFrameAPI) FirstFrameGate {
	gate := FirstFrameGate{window: window, api: api}
	if window == 0 {
		return gate
	}
	gate.transitionsDisabled = api.setDWMBoolean(window, firstFrameTransitionsForcedDisabled, true)
	gate.cloaked = api.setDWMBoolean(window, firstFrameCloak, true)
	return gate
}

// BeginFrameTransition starts an atomic visual update for a visible window.
// Unsupported DWM attributes safely fall back to a normal synchronous frame.
func BeginFrameTransition(window windows.Handle) FrameTransition {
	return beginFrameTransitionWith(window, defaultFirstFrameAPI)
}

func beginFrameTransitionWith(window windows.Handle, api firstFrameAPI) FrameTransition {
	transition := FrameTransition{window: window, api: api}
	if window == 0 {
		return transition
	}
	transition.transitionsDisabled = api.setDWMBoolean(window, firstFrameTransitionsForcedDisabled, true)
	transition.cloaked = api.setDWMBoolean(window, firstFrameCloak, true)
	return transition
}

// Commit synchronously paints the completed layout and then exposes it to
// DWM. A second identical frame after uncloak replaces any retained pre-cloak
// surface before a later input-driven paint can become visible.
func (t *FrameTransition) Commit(controls ...windows.Handle) error {
	if t == nil || t.window == 0 {
		return fmt.Errorf("frame-transition window is required")
	}
	if t.committed {
		return fmt.Errorf("frame transition already committed")
	}
	if t.transitionsDisabled {
		defer func() {
			t.api.setDWMBoolean(t.window, firstFrameTransitionsForcedDisabled, false)
			t.transitionsDisabled = false
		}()
	}
	t.api.present(t.window, controls...)
	if t.cloaked {
		if !t.api.setDWMBoolean(t.window, firstFrameCloak, false) {
			return fmt.Errorf("uncloak transitioned frame")
		}
		t.api.present(t.window, controls...)
	}
	t.committed = true
	return nil
}

// Reveal marks the HWND visible while it remains cloaked, commits one complete
// frame, and only then lets DWM expose it. The caller must destroy the window
// if Reveal returns an error.
func (g *FirstFrameGate) Reveal(options FirstFrameOptions) error {
	if g == nil || g.window == 0 {
		return fmt.Errorf("first-frame window is required")
	}
	if g.revealed {
		return fmt.Errorf("first frame already revealed")
	}
	if g.transitionsDisabled {
		defer g.api.setDWMBoolean(g.window, firstFrameTransitionsForcedDisabled, false)
	}
	g.api.showWindow(g.window)
	if options.RepeatShow {
		g.api.showWindow(g.window)
	}
	g.api.present(g.window, options.Controls...)
	if g.cloaked {
		if !g.api.setDWMBoolean(g.window, firstFrameCloak, false) {
			return fmt.Errorf("uncloak prepared first frame")
		}
		// DWM can retain the pre-cloak surface for one composition cycle even
		// though the hidden HWND was synchronously painted. Commit the identical
		// completed frame once more after uncloak so the first visible surface
		// never depends on a later mouse, caret or focus invalidation.
		g.api.present(g.window, options.Controls...)
	}
	g.revealed = true
	return nil
}

func setDWMBooleanAttribute(window windows.Handle, attribute uint32, enabled bool) bool {
	value := uint32(0)
	if enabled {
		value = 1
	}
	result, _, _ := pDwmSetWindowAttribute.Call(
		uintptr(window), uintptr(attribute),
		uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value),
	)
	return int32(result) >= 0
}
