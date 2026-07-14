//go:build ignore

// Command appicon produces the multi-resolution application ICO.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JeffioZ/idletrigger/tools/iconart"
	"github.com/JeffioZ/idletrigger/tools/iconfile"
)

func main() {
	dir := filepath.Join("build", "windows", "icons")
	if len(os.Args) == 2 {
		dir = os.Args[1]
	}
	if err := iconfile.WriteICO(filepath.Join(dir, "app.ico"), []int{16, 20, 24, 32, 40, 48, 64, 128, 256}, iconart.App); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
