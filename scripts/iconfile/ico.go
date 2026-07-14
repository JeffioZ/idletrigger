// Package iconfile writes PNG-backed multi-resolution Windows icon files.
package iconfile

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
)

// WriteICO draws one PNG frame per size and writes them as a Windows ICO.
func WriteICO(path string, sizes []int, draw func(int) image.Image) error {
	frames := make([][]byte, len(sizes))
	for i, size := range sizes {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, draw(size)); err != nil {
			return err
		}
		frames[i] = buffer.Bytes()
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := binary.Write(file, binary.LittleEndian, []uint16{0, 1, uint16(len(frames))}); err != nil {
		return err
	}
	offset := uint32(6 + 16*len(frames))
	for i, frame := range frames {
		side := byte(sizes[i])
		if sizes[i] == 256 {
			side = 0
		}
		if _, err := file.Write([]byte{side, side, 0, 0, 1, 0, 32, 0}); err != nil {
			return err
		}
		if err := binary.Write(file, binary.LittleEndian, uint32(len(frame))); err != nil {
			return err
		}
		if err := binary.Write(file, binary.LittleEndian, offset); err != nil {
			return err
		}
		offset += uint32(len(frame))
	}
	for _, frame := range frames {
		if _, err := file.Write(frame); err != nil {
			return err
		}
	}
	return nil
}
