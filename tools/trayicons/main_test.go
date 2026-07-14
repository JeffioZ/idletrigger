package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestICOFrameCoverageMatchesDeclaredResources(t *testing.T) {
	cases := map[string][]int{
		"app.ico":        {16, 20, 24, 32, 40, 48, 64, 128, 256},
		"tray-dark.ico":  {16, 20, 24, 32, 40, 48, 64},
		"tray-light.ico": {16, 20, 24, 32, 40, 48, 64},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			frames, err := readFrames(filepath.Join("..", "..", "build", "windows", "icons", name))
			if err != nil {
				t.Fatal(err)
			}
			got := make([]int, 0, len(frames))
			for _, frame := range frames {
				if frame.width != frame.height {
					t.Fatalf("non-square frame: %dx%d", frame.width, frame.height)
				}
				size := int(frame.width)
				if size == 0 {
					size = 256
				}
				got = append(got, size)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("frame coverage mismatch: got %v, want %v", got, want)
			}
		})
	}
}

func TestAppIconContainsTrayDPIFrames(t *testing.T) {
	frames, err := readFrames(filepath.Join("..", "..", "build", "windows", "icons", "app.ico"))
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
	for _, name := range []string{"app.ico", "tray-dark.ico", "tray-light.ico"} {
		t.Run(name, func(t *testing.T) {
			img := decodePNGFrame(t, filepath.Join("..", "..", "build", "windows", "icons", name), 16)
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
	dark := filepath.Join("..", "..", "build", "windows", "icons", "tray-dark.ico")
	light := filepath.Join("..", "..", "build", "windows", "icons", "tray-light.ico")
	for _, size := range []byte{16, 20, 24, 32, 40, 48, 64} {
		t.Run(string(rune(size)), func(t *testing.T) {
			darkImage := decodePNGFrame(t, dark, int(size))
			lightImage := decodePNGFrame(t, light, int(size))
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

func TestICOFramesArePNGsWithExpectedDimensions(t *testing.T) {
	for _, name := range []string{"app.ico", "tray-dark.ico", "tray-light.ico"} {
		t.Run(name, func(t *testing.T) {
			frames, err := readFrames(filepath.Join("..", "..", "build", "windows", "icons", name))
			if err != nil {
				t.Fatal(err)
			}
			for _, frame := range frames {
				size := int(frame.width)
				if size == 0 {
					size = 256
				}
				image := decodePNGFrame(t, filepath.Join("..", "..", "build", "windows", "icons", name), size)
				bounds := image.Bounds()
				if bounds.Dx() != int(size) || bounds.Dy() != int(size) {
					t.Fatalf("decoded frame dimensions = %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), size, size)
				}
			}
		})
	}
}

func decodePNGFrame(t *testing.T, path string, wantSize int) image.Image {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	for i := 0; i < count; i++ {
		offset := 6 + i*16
		frameSize := int(data[offset])
		if frameSize == 0 {
			frameSize = 256
		}
		frameHeight := int(data[offset+1])
		if frameHeight == 0 {
			frameHeight = 256
		}
		if frameSize != wantSize || frameHeight != wantSize {
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
