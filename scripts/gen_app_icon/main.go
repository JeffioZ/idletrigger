//go:build ignore

// Command gen_app_icon produces the multi-resolution application ICO.
package main

import (
	"fmt"
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
	if err := iconfile.WriteICO(filepath.Join(dir, "app.ico"), []int{16, 20, 24, 32, 40, 48, 64, 128, 256}, iconart.App); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
