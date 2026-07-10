package monitor

import (
	"testing"
	"time"
)

func TestNegativeWarningOffsetIsClamped(t *testing.T) {
	m := New(time.Minute, -time.Second, nil, nil, time.Second)
	if m.warningOffset != 0 {
		t.Fatalf("warning offset = %s, want 0", m.warningOffset)
	}
}

func TestStartStopCanRepeat(t *testing.T) {
	m := New(time.Hour, 0, nil, nil, time.Millisecond)
	for i := 0; i < 3; i++ {
		m.Start()
		m.Stop()
	}
}
