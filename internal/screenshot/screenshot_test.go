package screenshot

import (
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/JeffioZ/idletrigger/internal/popup"
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
	state := fixedSnapshot("zh-CN", popup.ThemeDark)
	if !state.IsChinese || state.Theme != popup.ThemeDark || !state.NoSleepEnabled || !state.IdleEnabled || !state.ThemeSwitchEnabled {
		t.Fatalf("fixture is not explicit: %#v", state)
	}
	if state.IPLocationEnabled || state.HotkeysEnabled || state.Theme == popup.ThemeFollowSystem {
		t.Fatalf("fixture permits external state: %#v", state)
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

func TestNormalizeClientUsesFixedCanvas(t *testing.T) {
	client := image.NewNRGBA(image.Rect(0, 0, 708, 891))
	normalized := normalizeClient(client)
	if got, want := normalized.Bounds().Size(), image.Pt(readmeClientWidth, readmeClientHeight); got != want {
		t.Fatalf("size = %v, want %v", got, want)
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
