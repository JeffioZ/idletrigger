//go:build windows && devtools

package screenshot

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/i18n"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/dpi"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/gdiplus"
	"github.com/JeffioZ/idletrigger/internal/ui/controlpanel"
)

// TestRoundedRectCapture renders the actual owner-draw path at the three DPI
// scales reviewed for the rounded-card migration. It writes files only when
// explicitly requested, so normal package tests remain side-effect free.
func TestRoundedRectCapture(t *testing.T) {
	dir := os.Getenv("IDLETRIGGER_ROUNDED_RECT_CAPTURE_OUT")
	if dir == "" {
		t.Skip("set IDLETRIGGER_ROUNDED_RECT_CAPTURE_OUT to capture rounded-card review images")
	}
	dpi.Enable()
	if os.Getenv("IDLETRIGGER_ROUNDED_RECT_CAPTURE_GDI_FALLBACK") != "1" {
		if !gdiplus.Start() {
			t.Skip("GDI+ is unavailable")
		}
		defer gdiplus.Shutdown()
	}
	scales := []struct {
		name  string
		value float64
	}{{"96", 1}, {"150", 1.5}, {"200", 2}}
	if requested := os.Getenv("IDLETRIGGER_ROUNDED_RECT_CAPTURE_SCALE"); requested != "" {
		var selected struct {
			name  string
			value float64
		}
		for _, scale := range scales {
			if scale.name == requested {
				selected = scale
				break
			}
		}
		if selected.name == "" {
			t.Fatalf("invalid IDLETRIGGER_ROUNDED_RECT_CAPTURE_SCALE=%q", requested)
		}
		scales = []struct {
			name  string
			value float64
		}{selected}
	}
	for _, scale := range scales {
		for _, language := range []string{"en", "zh-CN"} {
			for _, theme := range []controlpanel.Theme{controlpanel.ThemeLight, controlpanel.ThemeDark} {
				state := fixedSnapshot(language, theme)
				name := "panel-" + language + "-" + themeName(theme) + "-" + scale.name + ".png"
				path := filepath.Join(dir, name)
				err := controlpanel.Capture(state, func(key string) string { return i18n.T(language, key) }, scale.value, func(hwnd windows.Handle) error {
					window, err := printWindow(hwnd)
					if err != nil {
						return err
					}
					client, err := clientCrop(hwnd, window)
					if err != nil {
						return err
					}
					return writePNG(path, framePanelScreenshot(client, theme))
				})
				if err != nil {
					t.Fatalf("capture %s: %v", name, err)
				}
			}
		}
	}
}

func themeName(theme controlpanel.Theme) string {
	if theme == controlpanel.ThemeDark {
		return "dark"
	}
	return "light"
}
