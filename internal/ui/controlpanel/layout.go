package controlpanel

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
	if rowCount <= 0 {
		return 2 * surfaceInset
	}
	return 2*surfaceInset + rowCount*rowHeight + (rowCount-1)*rowGap
}

func menuRowOffset(index, rowHeight, rowGap, surfaceInset int) int {
	if index < 0 {
		index = 0
	}
	return surfaceInset + index*(rowHeight+rowGap)
}

func menuRowsFit(availableHeight, rowHeight, rowGap, surfaceInset int) int {
	contentHeight := availableHeight - 2*surfaceInset
	if contentHeight < rowHeight || rowHeight <= 0 {
		return 0
	}
	return (contentHeight + rowGap) / (rowHeight + rowGap)
}
