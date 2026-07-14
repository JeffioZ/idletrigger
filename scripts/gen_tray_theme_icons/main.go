// Command gen_tray_theme_icons produces purpose-built light and dark taskbar
// icons. It intentionally does not resize or recolor app.ico.
package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/JeffioZ/idletrigger/scripts/iconart"
	"github.com/JeffioZ/idletrigger/scripts/iconfile"
)

func main() {
	dir := "assets"
	if len(os.Args) == 2 {
		dir = os.Args[1]
	}
	sizes := []int{16, 20, 24, 32, 40, 48, 64}
	// dark is shown on a light taskbar; light is shown on a dark taskbar.
	if err := iconfile.WriteICO(filepath.Join(dir, "tray_icon_dark.ico"), sizes, func(size int) image.Image { return iconart.Tray(size, false) }); err != nil {
		fail(err)
	}
	if err := iconfile.WriteICO(filepath.Join(dir, "tray_icon_light.ico"), sizes, func(size int) image.Image { return iconart.Tray(size, true) }); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

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
