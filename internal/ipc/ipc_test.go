package ipc

import (
	"strings"
	"testing"
)

func TestPipeIdentity(t *testing.T) {
	name, err := pipeName()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, pipeBaseName+"-") {
		t.Fatalf("unexpected pipe name %q", name)
	}
	if sid, err := currentLogonSID(); err != nil || !strings.HasPrefix(sid, "S-") {
		t.Fatalf("invalid logon SID %q: %v", sid, err)
	}
}
