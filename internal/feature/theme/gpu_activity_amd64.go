package theme

import "unsafe"

type pdhFormattedItem struct {
	name    *uint16
	status  uint32
	padding uint32
	value   float64
}

func decodePDHFormattedItems(buffer []byte, count uint32) []pdhFormattedItem {
	if count == 0 || len(buffer) < int(count)*int(unsafe.Sizeof(pdhFormattedItem{})) {
		return nil
	}
	return unsafe.Slice((*pdhFormattedItem)(unsafe.Pointer(&buffer[0])), count)
}
