package power

import "testing"

func TestGetCapabilities_Concurrent(t *testing.T) {
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			GetCapabilities()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestGetStatus_NoPanic(t *testing.T) {
	s := GetStatus()
	_ = s.ACLine
	_ = s.Battery
}

func TestOnBattery_NoPanic(t *testing.T) {
	_ = OnBattery()
}
