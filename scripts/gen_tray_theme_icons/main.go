// Command gen_tray_theme_icons derives small tray icons from app.ico.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

type iconFrame struct {
	width, height byte
	image         image.Image
	raw           []byte
}

func main() {
	assetDir := "assets"
	if len(os.Args) == 2 {
		assetDir = os.Args[1]
	}
	frames, err := readFrames(filepath.Join(assetDir, "app.ico"))
	if err != nil {
		fail(err)
	}
	if err := writeICO(filepath.Join(assetDir, "tray_icon_dark.ico"), frames); err != nil {
		fail(err)
	}
	light := make([]iconFrame, len(frames))
	for i, frame := range frames {
		light[i] = iconFrame{width: frame.width, height: frame.height, image: lightTheme(frame.image)}
	}
	if err := writeICO(filepath.Join(assetDir, "tray_icon_light.ico"), light); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func readFrames(path string) ([]iconFrame, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 6 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return nil, fmt.Errorf("invalid ICO: %s", path)
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	frames := make([]iconFrame, 0, 4)
	for i := 0; i < count; i++ {
		entry := 6 + i*16
		if entry+16 > len(data) {
			return nil, fmt.Errorf("truncated ICO entry %d", i)
		}
		width, height := data[entry], data[entry+1]
		if width == 0 || height == 0 || width > 32 || height > 32 {
			continue
		}
		size := int(binary.LittleEndian.Uint32(data[entry+8 : entry+12]))
		offset := int(binary.LittleEndian.Uint32(data[entry+12 : entry+16]))
		if offset < 0 || size < 1 || offset+size > len(data) {
			return nil, fmt.Errorf("invalid ICO frame %d", i)
		}
		decoded, err := png.Decode(bytes.NewReader(data[offset : offset+size]))
		if err != nil {
			return nil, fmt.Errorf("decode ICO frame %d: %w", i, err)
		}
		frames = append(frames, iconFrame{width: width, height: height, image: decoded, raw: append([]byte(nil), data[offset:offset+size]...)})
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no tray-sized PNG frames in %s", path)
	}
	return frames, nil
}

func lightTheme(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			weight := boltWeight(uint8(r>>8), uint8(g>>8), uint8(b>>8))
			dst.SetNRGBA(x, y, mixLightIconColor(weight, uint8(a>>8)))
		}
	}
	return dst
}

func boltWeight(r, g, b byte) int {
	cyan := int(g)
	if b < g {
		cyan = int(b)
	}
	chroma := cyan - int(r)
	const backgroundChroma = 20
	const boltChroma = 130
	if chroma <= backgroundChroma {
		return 0
	}
	if chroma >= boltChroma {
		return 255
	}
	return (chroma - backgroundChroma) * 255 / (boltChroma - backgroundChroma)
}

func mixLightIconColor(weight int, alpha byte) color.NRGBA {
	const (
		backgroundR = 244
		backgroundG = 247
		backgroundB = 250
		boltR       = 0
		boltG       = 111
		boltB       = 139
	)
	mix := func(background, bolt int) byte {
		return byte((background*(255-weight) + bolt*weight) / 255)
	}
	return color.NRGBA{R: mix(backgroundR, boltR), G: mix(backgroundG, boltG), B: mix(backgroundB, boltB), A: alpha}
}

func writeICO(path string, frames []iconFrame) error {
	encoded := make([][]byte, len(frames))
	for i, frame := range frames {
		if frame.raw != nil {
			encoded[i] = frame.raw
			continue
		}
		var data bytes.Buffer
		if err := png.Encode(&data, frame.image); err != nil {
			return err
		}
		encoded[i] = data.Bytes()
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := binary.Write(file, binary.LittleEndian, uint16(0)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(len(frames))); err != nil {
		return err
	}
	offset := uint32(6 + len(frames)*16)
	for i, frame := range frames {
		entry := []byte{frame.width, frame.height, 0, 0, 1, 0, 32, 0}
		if _, err := file.Write(entry); err != nil {
			return err
		}
		if err := binary.Write(file, binary.LittleEndian, uint32(len(encoded[i]))); err != nil {
			return err
		}
		if err := binary.Write(file, binary.LittleEndian, offset); err != nil {
			return err
		}
		offset += uint32(len(encoded[i]))
	}
	for _, data := range encoded {
		if _, err := file.Write(data); err != nil {
			return err
		}
	}
	return nil
}
