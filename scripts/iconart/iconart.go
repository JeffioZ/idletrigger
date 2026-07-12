// Package iconart defines the pixel-oriented artwork shared by the release
// application icon and the two purpose-built Windows notification-area icons.
package iconart

import (
	"image"
	"image/color"
	"math"
)

type point struct{ x, y float64 }

// App draws the full application mark. Its material layers are deliberately
// restrained so that the lightning remains legible at the 16 px ICO frame.
func App(size int) image.Image {
	return render(size, func(x, y float64) color.NRGBA {
		m := 0.055
		if !roundedRect(x, y, m, m, 1-m, 1-m, 0.205) {
			return color.NRGBA{}
		}
		// Midnight surface with an intentionally low-contrast blue/purple rim.
		t := clamp01((x + y) * 0.5)
		base := lerp(color.NRGBA{14, 25, 45, 255}, color.NRGBA{24, 38, 68, 255}, t)
		if roundedRect(x, y, 0.105, 0.105, 0.895, 0.895, 0.155) {
			inner := lerp(color.NRGBA{18, 37, 69, 255}, color.NRGBA{11, 23, 43, 255}, y)
			base = over(base, inner, 0.72)
		}
		// A faint cool halo behind the mark, omitted visually at tiny scales.
		dx, dy := x-0.51, y-0.52
		d := math.Sqrt(dx*dx + dy*dy)
		if d < 0.34 {
			base = over(base, color.NRGBA{29, 120, 186, 255}, 0.10*(1-d/0.34))
		}
		bolt := appBolt()
		if inPolygon(x, y, bolt) {
			return lerp(color.NRGBA{41, 226, 255, 255}, color.NRGBA{39, 112, 255, 255}, y)
		}
		return base
	})
}

// Tray draws a separate, compact tile and high-contrast mark. It is
// intentionally not derived from App: Windows normally renders this at
// 16-20 px against an unknown taskbar color.
func Tray(size int, onDarkBackground bool) image.Image {
	return render(size, func(x, y float64) color.NRGBA {
		const margin = 0.055
		if !roundedRect(x, y, margin, margin, 1-margin, 1-margin, 0.205) {
			return color.NRGBA{}
		}
		var rim, tile, inset, boltTop, boltBottom color.NRGBA
		if onDarkBackground {
			// A pale tile remains visible on Windows' dark taskbar without relying
			// on a fragile glow or a one-pixel outline.
			rim = color.NRGBA{191, 221, 238, 255}
			tile = color.NRGBA{230, 242, 250, 255}
			inset = color.NRGBA{244, 250, 255, 255}
			boltTop = color.NRGBA{0, 95, 135, 255}
			boltBottom = color.NRGBA{0, 57, 96, 255}
		} else {
			rim = color.NRGBA{28, 65, 113, 255}
			tile = color.NRGBA{14, 30, 55, 255}
			inset = color.NRGBA{21, 47, 82, 255}
			boltTop = color.NRGBA{53, 231, 255, 255}
			boltBottom = color.NRGBA{33, 140, 255, 255}
		}
		if size >= 32 {
			// High-DPI notification areas can afford the same restrained material
			// hierarchy as the app icon: a rim, recessed surface and one soft
			// directional light. Small frames intentionally skip these layers.
			if !roundedRect(x, y, 0.082, 0.082, 0.918, 0.918, 0.165) {
				return lerp(rim, tile, y*0.30)
			}
			tile = lerp(tile, inset, 0.12*(1-y))
			if roundedRect(x, y, 0.125, 0.125, 0.875, 0.875, 0.125) {
				tile = lerp(tile, inset, 0.66)
			}
		} else if roundedRect(x, y, 0.105, 0.105, 0.895, 0.895, 0.145) {
			tile = lerp(tile, inset, 0.78)
		}
		bolt := trayBolt()
		if !inPolygon(x, y, bolt) {
			return tile
		}
		return lerp(boltTop, boltBottom, y)
	})
}

func appBolt() []point {
	return []point{{0.598, 0.178}, {0.315, 0.548}, {0.473, 0.548}, {0.424, 0.822}, {0.704, 0.437}, {0.545, 0.437}}
}

// This is wider and has fewer acute inner corners than appBolt, preventing
// the 16 px tray rendition from becoming a fuzzy miniature app icon.
func trayBolt() []point {
	return []point{{0.600, 0.132}, {0.255, 0.550}, {0.456, 0.550}, {0.393, 0.867}, {0.752, 0.426}, {0.548, 0.426}}
}

func render(size int, sample func(x, y float64) color.NRGBA) image.Image {
	const scale = 8
	large := image.NewNRGBA(image.Rect(0, 0, size*scale, size*scale))
	for y := 0; y < large.Bounds().Dy(); y++ {
		for x := 0; x < large.Bounds().Dx(); x++ {
			large.SetNRGBA(x, y, sample((float64(x)+0.5)/float64(size*scale), (float64(y)+0.5)/float64(size*scale)))
		}
	}
	return downsample(large, size, scale)
}

func downsample(src *image.NRGBA, size, scale int) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			// Accumulate in premultiplied-alpha space. Averaging straight RGB with
			// transparent black pixels creates dark fringes around curved ICO
			// edges when Windows composites the icon over a taskbar.
			var pr, pg, pb, a uint32
			for yy := 0; yy < scale; yy++ {
				for xx := 0; xx < scale; xx++ {
					c := src.NRGBAAt(x*scale+xx, y*scale+yy)
					pr += uint32(c.R) * uint32(c.A)
					pg += uint32(c.G) * uint32(c.A)
					pb += uint32(c.B) * uint32(c.A)
					a += uint32(c.A)
				}
			}
			n := uint32(scale * scale)
			if a == 0 {
				continue
			}
			// Convert back to straight-alpha NRGBA after averaging.
			dst.SetNRGBA(x, y, color.NRGBA{uint8(pr / a), uint8(pg / a), uint8(pb / a), uint8(a / n)})
		}
	}
	return dst
}

func roundedRect(x, y, left, top, right, bottom, radius float64) bool {
	if x >= left+radius && x <= right-radius || y >= top+radius && y <= bottom-radius {
		return x >= left && x <= right && y >= top && y <= bottom
	}
	cx := math.Max(left+radius, math.Min(x, right-radius))
	cy := math.Max(top+radius, math.Min(y, bottom-radius))
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= radius*radius
}

func inPolygon(x, y float64, polygon []point) bool {
	inside := false
	for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
		a, b := polygon[i], polygon[j]
		if (a.y > y) != (b.y > y) && x < (b.x-a.x)*(y-a.y)/(b.y-a.y)+a.x {
			inside = !inside
		}
	}
	return inside
}

func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	return color.NRGBA{mix(a.R, b.R, t), mix(a.G, b.G, t), mix(a.B, b.B, t), mix(a.A, b.A, t)}
}

func over(bottom, top color.NRGBA, opacity float64) color.NRGBA {
	return lerp(bottom, top, opacity)
}

func mix(a, b uint8, t float64) uint8 { return uint8(float64(a)*(1-t) + float64(b)*t + 0.5) }
func clamp01(v float64) float64       { return math.Max(0, math.Min(1, v)) }
