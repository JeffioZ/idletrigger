//go:build !devtools

package cli

import (
	"strings"
	"testing"
)

func TestReleaseUsageExcludesDeveloperCommands(t *testing.T) {
	if strings.Contains(usage("en"), "diagnostics") {
		t.Fatal("release usage includes diagnostics")
	}
}
