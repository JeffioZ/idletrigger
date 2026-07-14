//go:build tools

// Package tools anchors Go dependencies used only by ignored generator files.
package tools

import (
	_ "github.com/akavel/rsrc/binutil"
	_ "github.com/akavel/rsrc/coff"
	_ "github.com/akavel/rsrc/ico"
)
