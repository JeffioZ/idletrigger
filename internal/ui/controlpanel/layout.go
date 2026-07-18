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
