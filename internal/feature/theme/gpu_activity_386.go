package theme

import "unsafe"

// PDH uses the Windows SDK's default 8-byte structure packing on 32-bit
// builds, so both the nested value and its double begin on 8-byte boundaries.
type pdhFormattedItem struct {
	name      *uint16
	namePad   uint32
	status    uint32
	statusPad uint32
	value     float64
}

func decodePDHFormattedItems(buffer []byte, count uint32) []pdhFormattedItem {
	if count == 0 || len(buffer) < int(count)*int(unsafe.Sizeof(pdhFormattedItem{})) {
		return nil
	}
	return unsafe.Slice((*pdhFormattedItem)(unsafe.Pointer(&buffer[0])), count)
}
