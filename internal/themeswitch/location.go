package themeswitch

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"syscall"
)

type variant struct {
	VT         uint16
	_          [6]byte
	Val        [8]byte
}

const (
	vtEmpty    = 0
	vtR8       = 5
	vtR4       = 4
)

var (
	cachedLatLon struct {
		sync.Once
		lat, lon float64
	}
)

// GetLocation returns GPS coordinates via the Windows Location COM API.
// Cached after first call — subsequent calls return immediately.
// Falls back to 0,0 silently if location services are disabled.
func GetLocation() (float64, float64) {
	cachedLatLon.Do(func() {
		cachedLatLon.lat, cachedLatLon.lon = queryLocation()
	})
	return cachedLatLon.lat, cachedLatLon.lon
}

func queryLocation() (float64, float64) {
	// CLSID: LatLongReportFactory = {8A3CD7B2-E3E5-448D-83F8-E5384C2E4CF7}
	// This COM class provides the last known location.
	clsid := windows.GUID{
		Data1: 0x8A3CD7B2, Data2: 0xE3E5, Data3: 0x448D,
		Data4: [8]byte{0x83, 0xF8, 0xE5, 0x38, 0x4C, 0x2E, 0x4C, 0xF7},
	}
	iidIDispatch := windows.GUID{
		Data1: 0x00020400, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46},
	}

	// Initialize COM
	ole32 := windows.NewLazySystemDLL("ole32.dll")
	coInit := ole32.NewProc("CoInitializeEx")
	coInit.Call(0, 2) // COINIT_APARTMENTTHREADED
	defer ole32.NewProc("CoUninitialize").Call()

	// Create COM object
	coCreate := ole32.NewProc("CoCreateInstance")
	var disp uintptr
	r, _, _ := coCreate.Call(
		uintptr(unsafe.Pointer(&clsid)),
		0, // outer unknown
		1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidIDispatch)),
		uintptr(unsafe.Pointer(&disp)),
	)
	if r != 0 || disp == 0 {
		return 0, 0
	}
	defer releaseCom(disp)

	// Get "Latitude" property (DISPID 1)
	lat := getDispatchFloat(disp, 1)
	// Get "Longitude" property (DISPID 2)
	lon := getDispatchFloat(disp, 2)

	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0
	}
	return lat, lon
}

// getDispatchFloat invokes IDispatch::Invoke to read a float property.
func getDispatchFloat(disp uintptr, dispid int32) float64 {
	vtbl := *(**uintptr)(unsafe.Pointer(&disp))
	invokeSlot := (*[8]uintptr)(unsafe.Pointer(vtbl))[6] // IDispatch::Invoke = slot 6

	// Build DISPPARAMS
	type dispparams struct {
		args       *uint16
		namedArgs  *int32
		cArgs      uint32
		cNamedArgs uint32
	}
	var dp dispparams

	var result variant
	result.VT = vtEmpty

	var except uintptr
	var argErr uint32

	// DISPATCH_PROPERTYGET = 2, LOCALE_USER_DEFAULT = 0x0400
	r, _, _ := syscall.Syscall9(
		invokeSlot, 8,
		disp,                               // this
		uintptr(dispid),                     // dispid
		uintptr(unsafe.Pointer(&windows.GUID{})), // riid (IID_NULL)
		uintptr(0x0400),                     // lcid
		uintptr(2),                          // flags (DISPATCH_PROPERTYGET)
		uintptr(unsafe.Pointer(&dp)),        // params
		uintptr(unsafe.Pointer(&result)),    // result
		uintptr(unsafe.Pointer(&except)),    // exception
		uintptr(unsafe.Pointer(&argErr)),    // argErr
	)
	if r != 0 {
		return 0
	}
	if result.VT == vtR8 {
		return *(*float64)(unsafe.Pointer(&result.Val))
	}
	// Coerce to float if not already R8
	if result.VT == vtR4 {
		return float64(*(*float32)(unsafe.Pointer(&result.Val)))
	}
	return 0
}

func releaseCom(disp uintptr) {
	if disp == 0 {
		return
	}
	vtbl := *(**uintptr)(unsafe.Pointer(&disp))
	releaseSlot := (*[4]uintptr)(unsafe.Pointer(vtbl))[2] // IUnknown::Release = slot 2
	syscall.Syscall(releaseSlot, 1, disp, 0, 0)
}

