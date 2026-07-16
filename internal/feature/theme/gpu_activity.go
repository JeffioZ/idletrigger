package theme

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	pdhStatusSuccess  = 0
	pdhStatusMoreData = 0x800007d2
	pdhFormatDouble   = 0x00000200
	pdhFormatNoCap100 = 0x00008000

	foregroundGPUThreshold = 15.0
	foregroundGPUSamples   = 2
	foregroundGPUInterval  = 500 * time.Millisecond
)

var (
	themeEnvironmentPDH          = windows.NewLazySystemDLL("pdh.dll")
	pPDHOpenQuery                = themeEnvironmentPDH.NewProc("PdhOpenQueryW")
	pPDHAddEnglishCounter        = themeEnvironmentPDH.NewProc("PdhAddEnglishCounterW")
	pPDHCollectQueryData         = themeEnvironmentPDH.NewProc("PdhCollectQueryData")
	pPDHGetFormattedCounterArray = themeEnvironmentPDH.NewProc("PdhGetFormattedCounterArrayW")
	pPDHCloseQuery               = themeEnvironmentPDH.NewProc("PdhCloseQuery")
)

type gpuCounterSample struct {
	name  string
	usage float64
}

func foregroundProcessGPUActive(processID uint32, cancel <-chan struct{}) (bool, error) {
	if processID == 0 {
		return false, nil
	}
	path, err := windows.UTF16PtrFromString(`\GPU Engine(*)\Utilization Percentage`)
	if err != nil {
		return false, fmt.Errorf("prepare GPU activity counter: %w", err)
	}

	var query uintptr
	if status, _, _ := pPDHOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&query))); uint32(status) != pdhStatusSuccess {
		return false, pdhCallError("open GPU activity query", uint32(status))
	}
	defer pPDHCloseQuery.Call(query)

	var counter uintptr
	if status, _, _ := pPDHAddEnglishCounter.Call(query, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&counter))); uint32(status) != pdhStatusSuccess {
		return false, pdhCallError("add GPU activity counter", uint32(status))
	}
	if status, _, _ := pPDHCollectQueryData.Call(query); uint32(status) != pdhStatusSuccess {
		return false, pdhCallError("prime GPU activity query", uint32(status))
	}

	for sample := 0; sample < foregroundGPUSamples; sample++ {
		if err := waitForThemeEnvironmentSample(cancel, foregroundGPUInterval); err != nil {
			return false, err
		}
		if status, _, _ := pPDHCollectQueryData.Call(query); uint32(status) != pdhStatusSuccess {
			return false, pdhCallError("collect GPU activity", uint32(status))
		}
		items, err := readGPUCounterSamples(counter)
		if err != nil {
			return false, err
		}
		if currentForegroundProcessID() != processID {
			return false, nil
		}
		if foregroundProcess3DUsage(items, processID) < foregroundGPUThreshold {
			return false, nil
		}
	}
	return true, nil
}

func readGPUCounterSamples(counter uintptr) ([]gpuCounterSample, error) {
	const format = pdhFormatDouble | pdhFormatNoCap100
	var bufferSize uint32
	var itemCount uint32
	status, _, _ := pPDHGetFormattedCounterArray.Call(
		counter,
		format,
		uintptr(unsafe.Pointer(&bufferSize)),
		uintptr(unsafe.Pointer(&itemCount)),
		0,
	)
	if code := uint32(status); code != pdhStatusMoreData && code != pdhStatusSuccess {
		return nil, pdhCallError("size GPU activity data", code)
	}
	if bufferSize == 0 {
		return nil, nil
	}

	for attempt := 0; attempt < 3; attempt++ {
		buffer := make([]byte, bufferSize)
		itemCount = 0
		status, _, _ = pPDHGetFormattedCounterArray.Call(
			counter,
			format,
			uintptr(unsafe.Pointer(&bufferSize)),
			uintptr(unsafe.Pointer(&itemCount)),
			uintptr(unsafe.Pointer(&buffer[0])),
		)
		code := uint32(status)
		if code == pdhStatusMoreData {
			continue
		}
		if code != pdhStatusSuccess {
			return nil, pdhCallError("read GPU activity data", code)
		}

		formatted := decodePDHFormattedItems(buffer, itemCount)
		samples := make([]gpuCounterSample, 0, len(formatted))
		for _, item := range formatted {
			if item.name == nil || (item.status != 0 && item.status != 1) ||
				math.IsNaN(item.value) || math.IsInf(item.value, 0) {
				continue
			}
			samples = append(samples, gpuCounterSample{
				name:  windows.UTF16PtrToString(item.name),
				usage: item.value,
			})
		}
		return samples, nil
	}
	return nil, fmt.Errorf("read GPU activity data: counter list kept changing")
}

func foregroundProcess3DUsage(samples []gpuCounterSample, processID uint32) float64 {
	processToken := "pid_" + strconv.FormatUint(uint64(processID), 10) + "_"
	total := 0.0
	for _, sample := range samples {
		name := strings.ToLower(sample.name)
		if !strings.Contains(name, processToken) || !is3DGPUCounter(name) || sample.usage <= 0 {
			continue
		}
		total += sample.usage
	}
	return total
}

func is3DGPUCounter(counterName string) bool {
	counterName = strings.ToLower(counterName)
	return strings.Contains(counterName, "engtype_3d") || strings.Contains(counterName, "engtype_graphics")
}

func waitForThemeEnvironmentSample(cancel <-chan struct{}, duration time.Duration) error {
	if themeEnvironmentCheckCanceled(cancel) {
		return errThemeEnvironmentCheckCanceled
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-cancel:
		return errThemeEnvironmentCheckCanceled
	case <-timer.C:
		return nil
	}
}

func pdhCallError(operation string, status uint32) error {
	return fmt.Errorf("%s: PDH status 0x%08x", operation, status)
}
