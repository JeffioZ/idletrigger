package assets

import _ "embed"

// Icon data for the three tray-icon states.
var (
	//go:embed icon_default.ico
	IconDefault []byte

	//go:embed icon_monitor.ico
	IconMonitor []byte

	//go:embed icon_active.ico
	IconActive []byte
)
