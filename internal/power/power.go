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
		ACLineStatus       byte
		BatteryFlag        byte
		BatteryLifePercent byte
		SystemStatusFlag   byte
		BatteryLifeTime    uint32
		BatteryFullLifeTime uint32
	}
	var s sps
	proc.Call(uintptr(unsafe.Pointer(&s)))
	return Status{
		ACLine:   s.ACLineStatus == 1,
		Battery:  s.BatteryFlag != 128,
		Percent:  int(s.BatteryLifePercent),
		Charging: s.BatteryFlag&0x08 != 0,
	}
}

func OnBattery() bool { return !GetStatus().ACLine }
