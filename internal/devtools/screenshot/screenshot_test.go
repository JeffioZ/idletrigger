//go:build devtools

package screenshot

import (
	"errors"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/JeffioZ/idletrigger/internal/ui/controlpanel"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"single", []string{"screenshot", "--language", "en", "--theme", "dark", "--output", "out.png"}, false},
		{"all", []string{"screenshot", "--all", "--output", "images"}, false},
		{"missing language", []string{"screenshot", "--theme", "dark", "--output", "out.png"}, true},
		{"missing theme", []string{"screenshot", "--language", "en", "--output", "out.png"}, true},
		{"invalid language", []string{"screenshot", "--language", "fr", "--theme", "dark", "--output", "out.png"}, true},
		{"invalid theme", []string{"screenshot", "--language", "en", "--theme", "blue", "--output", "out.png"}, true},
		{"all conflict", []string{"screenshot", "--all", "--language", "en", "--output", "images"}, true},
		{"single png", []string{"screenshot", "--language", "en", "--theme", "light", "--output", "out.jpg"}, true},
		{"all output", []string{"screenshot", "--all"}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parse(test.args)
			if (err != nil) != test.wantErr {
				t.Fatalf("parse() error = %v, want error %v", err, test.wantErr)
			}
		})
	}
}

func TestAllJobsUseStableReadmeOrder(t *testing.T) {
	opts, err := parse([]string{"screenshot", "--all", "--output", "docs/images"})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := opts.jobs()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"panel-en-light.png", "panel-en-dark.png", "panel-zh-light.png", "panel-zh-dark.png"}
	if len(jobs) != len(want) {
		t.Fatalf("job count = %d", len(jobs))
	}
	for i, name := range want {
		if got := filepath.Base(jobs[i].path); got != name {
			t.Errorf("job %d = %q, want %q", i, got, name)
		}
	}
}

func TestFixedSnapshotIsExplicit(t *testing.T) {
	state := fixedSnapshot("zh-CN", controlpanel.ThemeDark)
	if !state.IsChinese || state.Theme != controlpanel.ThemeDark || !state.NoSleepEnabled || !state.IdleEnabled || !state.ThemeSwitchEnabled {
		t.Fatalf("fixture is not explicit: %#v", state)
	}
	if state.IPLocationEnabled || state.HotkeysEnabled || state.Theme == controlpanel.ThemeFollowSystem {
		t.Fatalf("fixture permits external state: %#v", state)
	}
}

func TestFixedSnapshotUsesSunriseSchedule(t *testing.T) {
	tests := []struct {
		language string
		want     string
	}{
		{"en", "Sunrise: 07:00 / Sunset: 19:00 · Estimated by timezone"},
		{"zh-CN", "日出：07:00 / 日落：19:00 · 按时区推算"},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			if got := fixedSnapshot(test.language, controlpanel.ThemeLight).ThemeSchedule; got != test.want {
				t.Fatalf("ThemeSchedule = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCropImage(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	cropped, err := cropImage(source, image.Rect(2, 3, 8, 9))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cropped.Bounds().Size(), image.Pt(6, 6); got != want {
		t.Fatalf("size = %v, want %v", got, want)
	}
	if _, err := cropImage(source, image.Rect(-1, 0, 4, 4)); err == nil {
		t.Fatal("expected bounds error")
	}
}

func TestFramePanelScreenshotAddsRoundedCornersAndShadow(t *testing.T) {
	panel := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			panel.SetNRGBA(x, y, color.NRGBA{R: 40, G: 80, B: 120, A: 255})
		}
	}
	framed := framePanelScreenshot(panel, controlpanel.ThemeLight)
	if got, want := framed.Bounds().Size(), image.Pt(40+2*screenshotFrameInset, 30+2*screenshotFrameInset); got != want {
		t.Fatalf("size = %v, want %v", got, want)
	}
	if got := framed.NRGBAAt(0, 0); got.A != 0 {
		t.Fatalf("outer corner = %#v, want transparent", got)
	}
	if got, want := framed.NRGBAAt(screenshotFrameInset+20, screenshotFrameInset+15), panel.NRGBAAt(20, 15); got != want {
		t.Fatalf("panel center = %#v, want %#v", got, want)
	}
	if got := framed.NRGBAAt(screenshotFrameInset+1, screenshotFrameInset+8); got.A == 0 || got.A == 255 {
		t.Fatalf("rounded edge = %#v, want antialiased alpha", got)
	}
	if got := framed.NRGBAAt(screenshotFrameInset+20, screenshotFrameInset+30+screenshotShadowOffset+2); got.A == 0 {
		t.Fatal("expected visible shadow below the panel")
	}
}

func TestFramePanelScreenshotUsesThemeAwareOutline(t *testing.T) {
	panel := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	light := framePanelScreenshot(panel, controlpanel.ThemeLight)
	dark := framePanelScreenshot(panel, controlpanel.ThemeDark)
	point := image.Pt(screenshotFrameInset-1, screenshotFrameInset+15)
	lightPixel, darkPixel := light.NRGBAAt(point.X, point.Y), dark.NRGBAAt(point.X, point.Y)
	if lightPixel.A == 0 || darkPixel.A == 0 {
		t.Fatalf("outline pixels = %#v, %#v; want visible", lightPixel, darkPixel)
	}
	if lightPixel == darkPixel {
		t.Fatalf("theme outline is identical: %#v", lightPixel)
	}
}

func TestPNGValidationAndAtomicWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "panel.png")
	first := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	if err := writePNG(path, first); err != nil {
		t.Fatal(err)
	}
	second := image.NewNRGBA(image.Rect(0, 0, 4, 5))
	if err := writePNG(path, second); err != nil {
		t.Fatal(err)
	}
	size, err := validatePNGFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if size != image.Pt(4, 5) {
		t.Fatalf("size = %v", size)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validatePNG(data); err != nil {
		t.Fatal(err)
	}
	if _, err := validatePNG([]byte("not a PNG")); err == nil {
		t.Fatal("expected invalid PNG error")
	}
}

func TestAtomicWriteRemovesTemporaryFileAfterFailure(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "panel.png")
	err := writePNGWith(path, image.NewNRGBA(image.Rect(0, 0, 1, 1)), func(io.Writer, image.Image) error {
		return errors.New("encode failed")
	})
	if err == nil {
		t.Fatal("expected encode error")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary files were not cleaned up: %#v", entries)
	}
}
