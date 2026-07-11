package monitor

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestNegativeWarningOffsetIsClamped(t *testing.T) {
	m := New(time.Minute, -time.Second, nil, nil, time.Second)
	if m.warningOffset != 0 {
		t.Fatalf("warning offset = %s, want 0", m.warningOffset)
	}
}

func TestElapsedSinceLastInputHandlesWrapAndSyntheticFutureTime(t *testing.T) {
	if got := elapsedSinceLastInput(25, ^uint32(0)-24); got != 50*time.Millisecond {
		t.Fatalf("wrapped elapsed = %s, want 50ms", got)
	}
	if got := elapsedSinceLastInput(100, 200); got != 0 {
		t.Fatalf("future input timestamp should reset idle, got %s", got)
	}
}

func TestStartStopCanRepeat(t *testing.T) {
	m := New(time.Hour, 0, nil, nil, time.Millisecond)
	for i := 0; i < 3; i++ {
		m.Start()
		m.Stop()
	}
}

func TestTriggerDoesNotDependOnWarning(t *testing.T) {
	triggered := make(chan struct{}, 1)
	m := New(30*time.Millisecond, 15*time.Millisecond, nil, func() { triggered <- struct{}{} }, time.Millisecond)
	m.idleDuration = func() (time.Duration, error) { return 2 * time.Minute, nil }
	m.inputTimestamp = func() (uint32, error) { return 1, nil }
	m.Start()
	defer m.Stop()

	select {
	case <-triggered:
	case <-time.After(time.Second):
		t.Fatal("trigger did not fire after the idle threshold")
	}
}

func TestStartDoesNotInheritExistingIdleDuration(t *testing.T) {
	triggered := make(chan struct{}, 1)
	m := New(80*time.Millisecond, 0, nil, func() { triggered <- struct{}{} }, time.Millisecond)
	m.idleDuration = func() (time.Duration, error) { return time.Hour, nil }
	m.inputTimestamp = func() (uint32, error) { return 1, nil }
	m.Start()
	defer m.Stop()

	select {
	case <-triggered:
		t.Fatal("monitor triggered using idle time from before Start")
	case <-time.After(30 * time.Millisecond):
	}

	select {
	case <-triggered:
	case <-time.After(time.Second):
		t.Fatal("monitor did not trigger after its own timeout window")
	}
}

func TestTriggerStartsFreshTimeoutWindow(t *testing.T) {
	triggered := make(chan struct{}, 2)
	m := New(40*time.Millisecond, 0, nil, func() { triggered <- struct{}{} }, time.Millisecond)
	m.idleDuration = func() (time.Duration, error) { return time.Hour, nil }
	m.inputTimestamp = func() (uint32, error) { return 1, nil }
	m.Start()
	defer m.Stop()

	select {
	case <-triggered:
	case <-time.After(time.Second):
		t.Fatal("first trigger did not fire")
	}
	select {
	case <-triggered:
		t.Fatal("trigger repeated without a fresh timeout window")
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case <-triggered:
	case <-time.After(time.Second):
		t.Fatal("fresh timeout window did not trigger")
	}
}

func TestActivityCallbackFollowsWarning(t *testing.T) {
	warned := make(chan struct{}, 1)
	activity := make(chan struct{}, 1)
	var input atomic.Uint32
	input.Store(1)
	m := New(40*time.Millisecond, 20*time.Millisecond, func() {
		input.Store(2)
		warned <- struct{}{}
	}, nil, time.Millisecond)
	m.idleDuration = func() (time.Duration, error) { return time.Hour, nil }
	m.inputTimestamp = func() (uint32, error) { return input.Load(), nil }
	m.SetOnActivity(func() { activity <- struct{}{} })
	m.Start()
	defer m.Stop()

	select {
	case <-warned:
	case <-time.After(time.Second):
		t.Fatal("warning did not fire")
	}
	select {
	case <-activity:
	case <-time.After(time.Second):
		t.Fatal("activity callback did not fire after warning")
	}
}
