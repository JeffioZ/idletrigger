package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestAppIconContainsTrayDPIFrames(t *testing.T) {
	frames, err := readFrames(filepath.Join("..", "..", "assets", "app.ico"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[byte]bool{16: false, 20: false, 24: false, 32: false, 40: false, 48: false, 64: false}
	for _, frame := range frames {
		if frame.width == frame.height {
			if _, ok := want[frame.width]; ok {
				want[frame.width] = true
			}
		}
	}
	for size, found := range want {
		if !found {
			t.Errorf("app.ico is missing the %dx%d tray frame", size, size)
		}
	}
}

func TestSmallFramesKeepTransparentCornersAndVisibleMarks(t *testing.T) {
	for _, name := range []string{"app.ico", "tray_icon_dark.ico", "tray_icon_light.ico"} {
		t.Run(name, func(t *testing.T) {
			img := decodePNGFrame(t, filepath.Join("..", "..", "assets", name), 16)
			for _, point := range [][2]int{{0, 0}, {15, 0}, {0, 15}, {15, 15}} {
				if _, _, _, alpha := img.At(point[0], point[1]).RGBA(); alpha != 0 {
					t.Fatalf("corner %v is not transparent", point)
				}
			}
			visible := 0
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					if _, _, _, alpha := img.At(x, y).RGBA(); alpha > 0x7fff {
						visible++
					}
				}
			}
			if visible < 16 {
				t.Fatalf("16px frame has too little visible coverage: %d pixels", visible)
			}
		})
	}
}

// The theme choice may change only colors. Keeping alpha masks byte-for-byte
// equal prevents a dark/light switch from subtly changing the tile outline,
// lightning geometry, padding, or antialiased edge pixels.
func TestTrayThemeFramesShareAlphaMask(t *testing.T) {
	dark := filepath.Join("..", "..", "assets", "tray_icon_dark.ico")
	light := filepath.Join("..", "..", "assets", "tray_icon_light.ico")
	for _, size := range []byte{16, 20, 24, 32, 40, 48, 64} {
		t.Run(string(rune(size)), func(t *testing.T) {
			darkImage := decodePNGFrame(t, dark, size)
			lightImage := decodePNGFrame(t, light, size)
			for y := 0; y < int(size); y++ {
				for x := 0; x < int(size); x++ {
					_, _, _, darkAlpha := darkImage.At(x, y).RGBA()
					_, _, _, lightAlpha := lightImage.At(x, y).RGBA()
					if darkAlpha != lightAlpha {
						t.Fatalf("alpha mismatch at (%d, %d): dark=%d light=%d", x, y, darkAlpha, lightAlpha)
					}
				}
			}
		})
	}
}

func decodePNGFrame(t *testing.T, path string, wantSize byte) image.Image {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	for i := 0; i < count; i++ {
		offset := 6 + i*16
		if data[offset] != wantSize || data[offset+1] != wantSize {
			continue
		}
		length := binary.LittleEndian.Uint32(data[offset+8 : offset+12])
		start := binary.LittleEndian.Uint32(data[offset+12 : offset+16])
		img, err := png.Decode(bytes.NewReader(data[start : start+length]))
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	t.Fatalf("missing %dx%d frame", wantSize, wantSize)
	return nil
}
