//go:build ignore

// Command gen_app_icon produces the multi-resolution application ICO.
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
	if err := writeICO(filepath.Join(dir, "app.ico"), []int{16, 20, 24, 32, 40, 48, 64, 128, 256}, iconart.App); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

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
	offset := uint32(6 + 16*len(frames))
	for i, frame := range frames {
		s := sizes[i]
		side := byte(s)
		if s == 256 {
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
