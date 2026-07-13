// Package screenshot hosts deterministic, side-effect-free captures of the
// real popup window. It never starts the tray or reads application config.
package screenshot

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/dpi"
	"github.com/JeffioZ/idletrigger/internal/i18n"
	"github.com/JeffioZ/idletrigger/internal/popup"
)

var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// Keep README images independent of the workstation or CI runner DPI while
// retaining the current 150% capture quality.
const readmeCaptureScale = 1.5

type options struct {
	all      bool
	language string
	theme    popup.Theme
	output   string
}

type job struct {
	language string
	theme    popup.Theme
	path     string
}

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes, Bits  uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	pGetWindowRect      = user32.NewProc("GetWindowRect")
	pGetClientRect      = user32.NewProc("GetClientRect")
	pClientToScreen     = user32.NewProc("ClientToScreen")
	pPrintWindow        = user32.NewProc("PrintWindow")
	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pDeleteDC           = gdi32.NewProc("DeleteDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
)

// IsCommand reports whether args select the isolated screenshot execution path.
func IsCommand(args []string) bool { return len(args) > 0 && args[0] == "screenshot" }

// Run captures deterministic README panel images without starting the normal app.
func Run(args []string) error {
	if len(args) == 2 && args[1] == "--help" {
		fmt.Fprintln(os.Stdout, usage())
		return nil
	}
	opts, err := parse(args)
	if err != nil {
		return err
	}
	dpi.Enable()
	run := func() error {
		jobs, err := opts.jobs()
		if err != nil {
			return err
		}
		capturedSizes := make(map[string]image.Point)
		for _, job := range jobs {
			state := fixedSnapshot(job.language, job.theme)
			err := popup.Capture(state, func(key string) string { return i18n.T(job.language, key) }, readmeCaptureScale, func(hwnd windows.Handle) error {
				img, err := printWindow(hwnd)
				if err != nil {
					return err
				}
				client, err := clientCrop(hwnd, img)
				if err != nil {
					return err
				}
				size := client.Bounds().Size()
				if previous, ok := capturedSizes[job.language]; ok && size != previous {
					return fmt.Errorf("inconsistent %s client capture size %dx%d; expected %dx%d", job.language, size.X, size.Y, previous.X, previous.Y)
				}
				capturedSizes[job.language] = size
				if err := writePNG(job.path, client); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("capture %s: %w", filepath.Base(job.path), err)
			}
		}
		return nil
	}
	return run()
}

func parse(args []string) (options, error) {
	if !IsCommand(args) {
		return options{}, fmt.Errorf("expected screenshot command")
	}
	var opts options
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--all":
			if opts.all {
				return options{}, fmt.Errorf("screenshot --all specified more than once")
			}
			opts.all = true
		case "--language", "--theme", "--output":
			if i+1 >= len(args) {
				return options{}, fmt.Errorf("screenshot option %q needs a value", args[i])
			}
			value := args[i+1]
			switch args[i] {
			case "--language":
				if opts.language != "" {
					return options{}, fmt.Errorf("screenshot language specified more than once")
				}
				if value != "en" && value != "zh-CN" {
					return options{}, fmt.Errorf("screenshot language must be en or zh-CN")
				}
				opts.language = value
			case "--theme":
				if opts.theme != popup.ThemeFollowSystem {
					return options{}, fmt.Errorf("screenshot theme specified more than once")
				}
				switch value {
				case "light":
					opts.theme = popup.ThemeLight
				case "dark":
					opts.theme = popup.ThemeDark
				default:
					return options{}, fmt.Errorf("screenshot theme must be light or dark")
				}
			case "--output":
				if opts.output != "" {
					return options{}, fmt.Errorf("screenshot output specified more than once")
				}
				opts.output = value
			}
			i++
		default:
			return options{}, fmt.Errorf("unknown screenshot option %q", args[i])
		}
	}
	if opts.all {
		if opts.language != "" || opts.theme != popup.ThemeFollowSystem {
			return options{}, fmt.Errorf("screenshot --all cannot be combined with --language or --theme")
		}
		if opts.output == "" {
			return options{}, fmt.Errorf("screenshot --all requires --output DIRECTORY")
		}
		return opts, nil
	}
	if opts.language == "" || opts.theme == popup.ThemeFollowSystem || opts.output == "" {
		return options{}, fmt.Errorf("single screenshot requires --language, --theme, and --output FILE.png")
	}
	if !strings.EqualFold(filepath.Ext(opts.output), ".png") {
		return options{}, fmt.Errorf("single screenshot --output must be a .png file")
	}
	return opts, nil
}

func usage() string {
	return "usage:\n  IdleTrigger.exe screenshot --all --output DIRECTORY\n  IdleTrigger.exe screenshot --language en|zh-CN --theme light|dark --output FILE.png"
}

func (opts options) jobs() ([]job, error) {
	if !opts.all {
		return []job{{language: opts.language, theme: opts.theme, path: opts.output}}, nil
	}
	return []job{
		{language: "en", theme: popup.ThemeLight, path: filepath.Join(opts.output, "panel-en-light.png")},
		{language: "en", theme: popup.ThemeDark, path: filepath.Join(opts.output, "panel-en-dark.png")},
		{language: "zh-CN", theme: popup.ThemeLight, path: filepath.Join(opts.output, "panel-zh-light.png")},
		{language: "zh-CN", theme: popup.ThemeDark, path: filepath.Join(opts.output, "panel-zh-dark.png")},
	}, nil
}

func fixedSnapshot(language string, theme popup.Theme) popup.State {
	chinese := language == "zh-CN"
	schedule := fmt.Sprintf(i18n.T(language, "theme_schedule_sunrise_format"), "07:00", "19:00")
	schedule = fmt.Sprintf(i18n.T(language, "theme_schedule_source_format"), schedule, i18n.T(language, "theme_location_utc_offset"))
	return popup.State{NoSleepEnabled: true, ProcessWatchEnabled: false, IdleEnabled: true, IdleWarningEnabled: true, IdleEnhancedMonitor: false, IdleTimeout: 30, IdleWarningSeconds: 30, IdleAction: "lock", ThemeSwitchEnabled: true, DarkOnBattery: true, SkipFullscreen: true, IPLocationEnabled: false, HotkeysEnabled: false, AutostartEnabled: true, LoggingEnabled: true, IsChinese: chinese, ThemeSchedule: schedule, AppVersion: "dev", Theme: theme}
}

func printWindow(hwnd windows.Handle) (*image.NRGBA, error) {
	var bounds rect
	if ok, _, err := pGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&bounds))); ok == 0 {
		return nil, fmt.Errorf("GetWindowRect: %w", err)
	}
	width, height := bounds.Right-bounds.Left, bounds.Bottom-bounds.Top
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid capture size %dx%d", width, height)
	}
	dc, _, err := pCreateCompatibleDC.Call(0)
	if dc == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC: %w", err)
	}
	defer pDeleteDC.Call(dc)
	info := bitmapInfo{Header: bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: width, Height: -height, Planes: 1, Bits: 32}}
	var pixels unsafe.Pointer
	bitmap, _, err := pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bitmap == 0 || pixels == nil {
		return nil, fmt.Errorf("CreateDIBSection: %w", err)
	}
	old, _, _ := pSelectObject.Call(dc, bitmap)
	defer func() { pSelectObject.Call(dc, old); pDeleteObject.Call(bitmap) }()
	const pwRenderFullContent = 0x00000002
	if ok, _, err := pPrintWindow.Call(uintptr(hwnd), dc, pwRenderFullContent); ok == 0 {
		return nil, fmt.Errorf("PrintWindow: %w", err)
	}
	stride := int(width) * 4
	src := unsafe.Slice((*byte)(pixels), stride*int(height))
	img := image.NewNRGBA(image.Rect(0, 0, int(width), int(height)))
	for y := 0; y < int(height); y++ {
		for x := 0; x < int(width); x++ {
			offset := y*stride + x*4
			img.Pix[offset], img.Pix[offset+1], img.Pix[offset+2], img.Pix[offset+3] = src[offset+2], src[offset+1], src[offset], 0xff
		}
	}
	return img, nil
}

func clientCrop(hwnd windows.Handle, captured image.Image) (*image.NRGBA, error) {
	var windowRect, clientRect rect
	if ok, _, err := pGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&windowRect))); ok == 0 {
		return nil, fmt.Errorf("GetWindowRect for client crop: %w", err)
	}
	if ok, _, err := pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&clientRect))); ok == 0 {
		return nil, fmt.Errorf("GetClientRect for client crop: %w", err)
	}
	clientOrigin := point{}
	if ok, _, err := pClientToScreen.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&clientOrigin))); ok == 0 {
		return nil, fmt.Errorf("ClientToScreen for client crop: %w", err)
	}
	crop := image.Rect(int(clientOrigin.X-windowRect.Left), int(clientOrigin.Y-windowRect.Top), int(clientOrigin.X-windowRect.Left+clientRect.Right-clientRect.Left), int(clientOrigin.Y-windowRect.Top+clientRect.Bottom-clientRect.Top))
	return cropImage(captured, crop)
}

func cropImage(source image.Image, crop image.Rectangle) (*image.NRGBA, error) {
	if crop.Empty() || !crop.In(source.Bounds()) {
		return nil, fmt.Errorf("client crop %v is outside captured bitmap %v", crop, source.Bounds())
	}
	result := image.NewNRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	for y := crop.Min.Y; y < crop.Max.Y; y++ {
		for x := crop.Min.X; x < crop.Max.X; x++ {
			result.Set(x-crop.Min.X, y-crop.Min.Y, source.At(x, y))
		}
	}
	return result, nil
}

func writePNG(path string, img image.Image) error {
	return writePNGWith(path, img, encodePNG)
}

func writePNGWith(path string, img image.Image, encode func(io.Writer, image.Image) error) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create screenshot output directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".idletrigger-screenshot-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary screenshot PNG: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := encode(temporary, img); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close screenshot PNG: %w", err)
	}
	if _, err := validatePNGFile(temporaryPath); err != nil {
		return fmt.Errorf("validate screenshot PNG before replace: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace screenshot PNG: %w", err)
	}
	committed = true
	return nil
}

func encodePNG(w io.Writer, img image.Image) error {
	if err := png.Encode(w, img); err != nil {
		return fmt.Errorf("encode screenshot PNG: %w", err)
	}
	return nil
}

func validatePNGFile(path string) (image.Point, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return image.Point{}, fmt.Errorf("read PNG: %w", err)
	}
	return validatePNG(data)
}

func validatePNG(data []byte) (image.Point, error) {
	if len(data) == 0 || !bytes.HasPrefix(data, pngSignature) {
		return image.Point{}, fmt.Errorf("invalid PNG signature")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Point{}, fmt.Errorf("decode PNG: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return image.Point{}, fmt.Errorf("invalid PNG size %dx%d", config.Width, config.Height)
	}
	return image.Pt(config.Width, config.Height), nil
}
