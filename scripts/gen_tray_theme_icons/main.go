// Command gen_tray_theme_icons produces purpose-built light and dark taskbar
// icons. It intentionally does not resize or recolor app.ico.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/JeffioZ/idletrigger/scripts/iconart"
)

func main() {
	dir := "assets"
	if len(os.Args) == 2 {
		dir = os.Args[1]
	}
	sizes := []int{16, 20, 24, 32, 40, 48, 64}
	// dark is shown on a light taskbar; light is shown on a dark taskbar.
	if err := writeICO(filepath.Join(dir, "tray_icon_dark.ico"), sizes, func(size int) image.Image { return iconart.Tray(size, false) }); err != nil {
		fail(err)
	}
	if err := writeICO(filepath.Join(dir, "tray_icon_light.ico"), sizes, func(size int) image.Image { return iconart.Tray(size, true) }); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func writeICO(path string, sizes []int, draw func(int) image.Image) error {
	frames := make([][]byte, len(sizes))
	for i, size := range sizes {
		var b bytes.Buffer
		if err := png.Encode(&b, draw(size)); err != nil {
			return err
		}
		frames[i] = b.Bytes()
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := binary.Write(f, binary.LittleEndian, []uint16{0, 1, uint16(len(frames))}); err != nil {
		return err
	}
	offset := uint32(6 + len(frames)*16)
	for i, frame := range frames {
		side := byte(sizes[i])
		if sizes[i] == 256 {
			side = 0
		}
		if _, err := f.Write([]byte{side, side, 0, 0, 1, 0, 32, 0}); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, uint32(len(frame))); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, offset); err != nil {
			return err
		}
		offset += uint32(len(frame))
	}
	for _, frame := range frames {
		if _, err := f.Write(frame); err != nil {
			return err
		}
	}
	return nil
}

// iconFrame and readFrames keep the frame-coverage test independent from the
// artwork implementation. The decoder only needs ICO directory metadata.
type iconFrame struct{ width, height byte }

func readFrames(path string) ([]iconFrame, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 6 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return nil, fmt.Errorf("invalid ICO: %s", path)
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if len(data) < 6+count*16 {
		return nil, fmt.Errorf("truncated ICO: %s", path)
	}
	frames := make([]iconFrame, 0, count)
	for i := 0; i < count; i++ {
		offset := 6 + i*16
		frames = append(frames, iconFrame{width: data[offset], height: data[offset+1]})
	}
	return frames, nil
}
