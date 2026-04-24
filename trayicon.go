package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// createTrayIcon generates a 32x32 PNG icon for the system tray.
// Design: a central routing hub with radiating spokes to model nodes.
func createTrayIcon() []byte {
	const s = 32
	img := image.NewRGBA(image.Rect(0, 0, s, s))

	// Colors
	bg := color.RGBA{15, 23, 42, 255}
	hubFill := color.RGBA{30, 41, 59, 255}
	hubBorder := color.RGBA{59, 130, 246, 255}
	hubInner := color.RGBA{96, 165, 250, 255}
	cyan := color.RGBA{6, 182, 212, 255}
	violet := color.RGBA{139, 92, 246, 255}
	amber := color.RGBA{245, 158, 11, 255}
	emerald := color.RGBA{16, 185, 129, 255}

	// Fill background
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			img.Set(x, y, bg)
		}
	}

	// Helper: draw a filled circle
	drawCircle := func(cx, cy, r int, col color.RGBA) {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if dx*dx+dy*dy <= r*r {
					px, py := cx+dx, cy+dy
					if px >= 0 && px < s && py >= 0 && py < s {
						img.Set(px, py, col)
					}
				}
			}
		}
	}

	// Helper: draw a pixel line (Bresenham-ish, just fill nearby)
	drawSpoke := func(x1, y1, x2, y2 int, col color.RGBA, thickness int) {
		dx := math.Abs(float64(x2 - x1))
		dy := math.Abs(float64(y2 - y1))
		steps := int(math.Max(dx, dy))
		if steps == 0 {
			steps = 1
		}
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)
			x := int(float64(x1) + t*float64(x2-x1))
			y := int(float64(y1) + t*float64(y2-y1))
			for ty := -thickness; ty <= thickness; ty++ {
				for tx := -thickness; tx <= thickness; tx++ {
					if tx*tx+ty*ty <= thickness*thickness {
						px, py := x+tx, y+ty
						if px >= 0 && px < s && py >= 0 && py < s {
							img.Set(px, py, col)
						}
					}
				}
			}
		}
	}

	// Center: hub = (16, 16)
	cx, cy := 16, 16

	// Outer orbit ring (dashed-ish)
	for a := 0; a < 360; a += 12 {
		rad := float64(a) * math.Pi / 180
		x := cx + int(13*math.Cos(rad))
		y := cy + int(13*math.Sin(rad))
		img.Set(x, y, color.RGBA{59, 130, 246, 60})
	}

	// Spokes: hub to nodes
	drawSpoke(cx, cy, cx, 6, cyan, 1)          // top
	drawSpoke(cx, cy, 26, cy, violet, 1)      // right
	drawSpoke(cx, cy, cx, 26, emerald, 1)     // bottom
	drawSpoke(cx, cy, 6, cy, color.RGBA{96, 165, 250, 255}, 1) // left

	// Outer nodes
	drawCircle(cx, 6, 3, cyan)       // top
	drawCircle(26, cy, 3, violet)   // right
	drawCircle(cx, 26, 2, emerald)  // bottom
	drawCircle(6, cy, 2, color.RGBA{96, 165, 250, 255}) // left

	// Diagonal spoke dots
	drawSpoke(cx, cy, 22, 10, amber, 1)
	drawCircle(22, 10, 2, amber)

	// Center hub outer ring
	drawCircle(cx, cy, 7, hubBorder)
	// Center hub inner
	drawCircle(cx, cy, 5, hubFill)
	drawCircle(cx, cy, 3, hubInner)

	// 4-way arrow symbol in center (tiny cross arrows)
	// Up arrow
	for dy := -2; dy <= 0; dy++ {
		for dx := -1; dx <= 1; dx++ {
			img.Set(cx+dx, cy+dy, hubInner)
		}
	}
	// Down arrow
	for dy := 0; dy <= 2; dy++ {
		for dx := -1; dx <= 1; dx++ {
			img.Set(cx+dx, cy+dy, hubInner)
		}
	}
	// Left arrow
	for dx := -2; dx <= 0; dx++ {
		for dy := -1; dy <= 1; dy++ {
			img.Set(cx+dx, cy+dy, hubInner)
		}
	}
	// Right arrow
	for dx := 0; dx <= 2; dx++ {
		for dy := -1; dy <= 1; dy++ {
			img.Set(cx+dx, cy+dy, hubInner)
		}
	}
	// Center dot
	img.Set(cx, cy, color.RGBA{59, 130, 246, 255})

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
