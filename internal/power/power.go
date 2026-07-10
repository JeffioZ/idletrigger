package power

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Capabilities struct {
	SleepAvailable     bool
	HibernateAvailable bool
}

type Status struct {
	ACLine   bool
	Battery  bool
	Percent  int
	Charging bool
	Valid    bool
}

var (
	cachedCaps Capabilities
	capsOnce   sync.Once
)

func GetCapabilities() Capabilities {
	capsOnce.Do(func() {
		var caps Capabilities
		powrprof := windows.NewLazySystemDLL("powrprof.dll")
		callNt := powrprof.NewProc("CallNtPowerInformation")
		const bufSize = 128
		var buf [bufSize]byte
		const systemPowerCaps = 4
		ret, _, _ := callNt.Call(uintptr(systemPowerCaps), 0, 0,
			uintptr(unsafe.Pointer(&buf[0])), uintptr(bufSize))
		if ret != 0 {
			cachedCaps = caps
			return
		}
		caps.SleepAvailable = buf[5] != 0 || buf[20] != 0
		caps.HibernateAvailable = buf[6] != 0 && buf[8] != 0
		cachedCaps = caps
	})
	return cachedCaps
}

func GetStatus() Status {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := kernel32.NewProc("GetSystemPowerStatus")
	type sps struct {
		ACLineStatus        byte
		BatteryFlag         byte
		BatteryLifePercent  byte
		SystemStatusFlag    byte
		BatteryLifeTime     uint32
		BatteryFullLifeTime uint32
	}
	var s sps
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&s)))
	if r == 0 {
		return Status{}
	}
	batteryKnown := s.BatteryFlag != 255
	percent := int(s.BatteryLifePercent)
	if s.BatteryLifePercent == 255 {
		percent = -1
	}
	return Status{
		ACLine:   s.ACLineStatus == 1,
		Battery:  batteryKnown && s.BatteryFlag != 128,
		Percent:  percent,
		Charging: s.BatteryFlag&0x08 != 0,
		Valid:    s.ACLineStatus != 255 && batteryKnown,
	}
}

func OnBattery() bool {
	status := GetStatus()
	return status.Valid && status.Battery && !status.ACLine
}
