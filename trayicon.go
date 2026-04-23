package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// createTrayIcon generates a 32x32 PNG icon for the system tray.
func createTrayIcon() []byte {
	const s = 32
	img := image.NewRGBA(image.Rect(0, 0, s, s))

	bg := color.RGBA{15, 23, 42, 255}
	border := color.RGBA{59, 130, 246, 255}
	center := color.RGBA{59, 130, 246, 255}
	node1 := color.RGBA{6, 182, 212, 255}
	node2 := color.RGBA{139, 92, 246, 255}
	node3 := color.RGBA{245, 158, 11, 255}

	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			dx, dy := x-16, y-16
			dist2 := dx*dx + dy*dy

			switch {
			case dist2 <= 11*11:
				// Inside circle - background
				img.Set(x, y, bg)
				// Center dot
				if dx*dx+dy*dy <= 3*3 {
					img.Set(x, y, center)
				}
			case dist2 <= 13*13:
				// Border ring
				img.Set(x, y, border)
			default:
				// Transparent
			}

			// Node dots
			nodes := []struct {
				cx, cy int
				col    color.RGBA
			}{
				{7, 7, node1}, {24, 7, node2},
				{7, 24, node2}, {24, 24, node3},
			}
			for _, n := range nodes {
				ndx, ndy := x-n.cx, y-n.cy
				if ndx*ndx+ndy*ndy <= 2*2 {
					img.Set(x, y, n.col)
				}
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
