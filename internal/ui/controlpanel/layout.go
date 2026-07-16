package controlpanel

import "github.com/JeffioZ/idletrigger/internal/ui/nativeform"

// logicalBounds is the DPI-independent layout result retained for each native
// child control. It is consumed during creation and WM_DPICHANGED resizing.
type logicalBounds struct{ x, y, width, height int }

func splitRow(totalWidth, count, gap int) int {
	if count <= 0 {
		return 0
	}
	return (totalWidth - (count-1)*gap) / count
}

func menuHeight(rowCount, rowHeight, rowGap, surfaceInset int) int {
	return nativeform.MenuHeight(rowCount, rowHeight, rowGap, surfaceInset)
}

func menuRowOffset(index, rowHeight, rowGap, surfaceInset int) int {
	return nativeform.MenuRowOffset(index, rowHeight, rowGap, surfaceInset)
}

func menuRowsFit(availableHeight, rowHeight, rowGap, surfaceInset int) int {
	return nativeform.MenuRowsFit(availableHeight, rowHeight, rowGap, surfaceInset)
}
