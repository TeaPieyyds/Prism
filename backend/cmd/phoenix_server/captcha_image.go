package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
)

// 8x13 bitmap font for digits 0-9 and +, -, =, ?
var font8x13 = map[rune][13]uint8{
	'0': {
		0b01111100,
		0b11111110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11111110,
		0b01111100,
	},
	'1': {
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
	},
	'2': {
		0b01111100,
		0b11111110,
		0b11000110,
		0b00000110,
		0b00000110,
		0b00001100,
		0b00011000,
		0b00110000,
		0b01100000,
		0b11000000,
		0b11000000,
		0b11111110,
		0b11111110,
	},
	'3': {
		0b01111100,
		0b11111110,
		0b11000110,
		0b00000110,
		0b00000110,
		0b00011100,
		0b00011100,
		0b00000110,
		0b00000110,
		0b00000110,
		0b11000110,
		0b11111110,
		0b01111100,
	},
	'4': {
		0b00001100,
		0b00011100,
		0b00111100,
		0b01101100,
		0b11001100,
		0b11001100,
		0b11001100,
		0b11111110,
		0b11111110,
		0b00001100,
		0b00001100,
		0b00001100,
		0b00001100,
	},
	'5': {
		0b11111110,
		0b11111110,
		0b11000000,
		0b11000000,
		0b11000000,
		0b11111100,
		0b11111110,
		0b00000110,
		0b00000110,
		0b00000110,
		0b11000110,
		0b11111110,
		0b01111100,
	},
	'6': {
		0b01111100,
		0b11111110,
		0b11000110,
		0b11000000,
		0b11000000,
		0b11111100,
		0b11111110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11111110,
		0b01111100,
	},
	'7': {
		0b11111110,
		0b11111110,
		0b00000110,
		0b00000110,
		0b00001100,
		0b00001100,
		0b00011000,
		0b00011000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00110000,
	},
	'8': {
		0b01111100,
		0b11111110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b01111100,
		0b01111100,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11111110,
		0b01111100,
	},
	'9': {
		0b01111100,
		0b11111110,
		0b11000110,
		0b11000110,
		0b11000110,
		0b11111110,
		0b01111110,
		0b00000110,
		0b00000110,
		0b00000110,
		0b11000110,
		0b11111110,
		0b01111100,
	},
	'+': {
		0b00000000,
		0b00000000,
		0b00011000,
		0b00011000,
		0b00011000,
		0b11111111,
		0b11111111,
		0b00011000,
		0b00011000,
		0b00011000,
		0b00000000,
		0b00000000,
		0b00000000,
	},
	'-': {
		0b00000000,
		0b00000000,
		0b00000000,
		0b00000000,
		0b00000000,
		0b11111111,
		0b11111111,
		0b00000000,
		0b00000000,
		0b00000000,
		0b00000000,
		0b00000000,
		0b00000000,
	},
	'=': {
		0b00000000,
		0b00000000,
		0b00000000,
		0b00000000,
		0b11111111,
		0b00000000,
		0b00000000,
		0b11111111,
		0b00000000,
		0b00000000,
		0b00000000,
		0b00000000,
		0b00000000,
	},
	'?': {
		0b01111100,
		0b11111110,
		0b11000110,
		0b00000110,
		0b00001110,
		0b00011100,
		0b00110000,
		0b00110000,
		0b00110000,
		0b00000000,
		0b00110000,
		0b00110000,
		0b00110000,
	},
}

// rotatePixel rotates point (px,py) around (cx,cy) by angle radians using nearest-neighbor.
func rotatePixel(px, py int, cx, cy float64, sinA, cosA float64) (int, int) {
	dx := float64(px) - cx
	dy := float64(py) - cy
	return int(cx + dx*cosA - dy*sinA + 0.5), int(cy + dx*sinA + dy*cosA + 0.5)
}

func drawCaptcha(numA, numB int, op string) string {
	const (
		fontScale = 4 // pixels per font bit
		fontW     = 8
		fontH     = 13
		padX      = 20
		padY      = 16
	)

	text := fmt.Sprintf("%d%s%d=?", numA, op, numB)
	runes := []rune(text)
	// Each char gets variable width: 8*scale + random extra
	charPitches := make([]int, len(runes))
	totalW := padX * 2
	for i := range runes {
		pitch := fontW*fontScale + rand.Intn(6) - 2
		if pitch < fontW*fontScale-1 {
			pitch = fontW*fontScale - 1
		}
		charPitches[i] = pitch
		totalW += pitch
	}
	totalH := padY*2 + fontH*fontScale

	img := image.NewRGBA(image.Rect(0, 0, totalW, totalH))

	// subtle gradient background
	for y := 0; y < totalH; y++ {
		t := float64(y) / float64(totalH)
		r := uint8(0xF5 - t*15)
		g := uint8(0xF0 - t*12)
		b := uint8(0xE8 - t*10)
		for x := 0; x < totalW; x++ {
			img.Set(x, y, color.RGBA{r, g, b, 0xFF})
		}
	}

	// ── Aggressive noise ──
	// Thick arcs
	for i := 0; i < 6; i++ {
		cx := rand.Intn(totalW)
		cy := rand.Intn(totalH)
		r := rand.Intn(60) + 20
		c := color.RGBA{139, 122, 97, uint8(15 + rand.Intn(25))}
		addArc(img, cx, cy, r, c)
	}

	// Noise lines across the whole image (some thick)
	for i := 0; i < 8; i++ {
		x1, y1 := rand.Intn(totalW), rand.Intn(totalH)
		x2, y2 := rand.Intn(totalW), rand.Intn(totalH)
		c := color.RGBA{25, 200, 185, uint8(18 + rand.Intn(20))}
		thickLine := rand.Intn(3)
		for t := 0; t <= thickLine; t++ {
			addLine(img, x1+t, y1, x2+t, y2, c)
		}
	}

	// Dense dots
	for i := 0; i < 80; i++ {
		c := color.RGBA{93, 75, 54, uint8(15 + rand.Intn(35))}
		img.Set(rand.Intn(totalW), rand.Intn(totalH), c)
	}

	// ── Draw characters with per-char rotation + wave ──
	xPos := padX
	for ci, ch := range runes {
		glyph, ok := font8x13[ch]
		if !ok {
			glyph = font8x13['0']
		}

		// Per-character random rotation: -20° to +20°
		angle := (rand.Float64() - 0.5) * 0.7
		sinA, cosA := math.Sin(angle), math.Cos(angle)

		// Random vertical wave offset
		waveOff := int(math.Sin(float64(ci)*1.7) * 5)

		// Character center for rotation
		cx := float64(xPos + fontW*fontScale/2)
		cy := float64(padY + fontH*fontScale/2 + waveOff)

		// Random color per character (warm browns)
		textColor := color.RGBA{
			uint8(80 + rand.Intn(100)),
			uint8(60 + rand.Intn(50)),
			uint8(30 + rand.Intn(40)),
			0xFF,
		}
		// 20% chance of a contrasting color
		if rand.Intn(5) == 0 {
			textColor = color.RGBA{
				uint8(20 + rand.Intn(60)),
				uint8(100 + rand.Intn(80)),
				uint8(100 + rand.Intn(80)),
				0xFF,
			}
		}

		for row := 0; row < fontH; row++ {
			for col := 0; col < fontW; col++ {
				if glyph[row]&(1<<(7-col)) == 0 {
					continue
				}
				// Base pixel position
				baseX := xPos + col*fontScale
				baseY := padY + row*fontScale + waveOff

				// Draw scaled pixel with rotation
				for dy := 0; dy < fontScale; dy++ {
					for dx := 0; dx < fontScale; dx++ {
						px, py := baseX+dx, baseY+dy
						// Rotate around character center
						rx, ry := rotatePixel(px, py, cx, cy, sinA, cosA)
						// Add tiny jitter
						rx += rand.Intn(3) - 1
						ry += rand.Intn(3) - 1
						if rx >= 0 && rx < totalW && ry >= 0 && ry < totalH {
							// Anti-aliasing: slight opacity at edges
							alpha := uint8(200 + rand.Intn(56))
							img.Set(rx, ry, color.RGBA{
								textColor.R, textColor.G, textColor.B, alpha,
							})
						}
					}
				}
			}
		}
		xPos += charPitches[ci]
	}

	// ── Post-processing: additional noise over text ──
	for i := 0; i < 4; i++ {
		x1, y1 := rand.Intn(totalW), rand.Intn(totalH)
		x2, y2 := rand.Intn(totalW), rand.Intn(totalH)
		c := color.RGBA{200, 100, 100, uint8(10 + rand.Intn(15))}
		addLine(img, x1, y1, x2, y2, c)
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func addArc(img *image.RGBA, cx, cy, r int, c color.Color) {
	for x := -r; x <= r; x++ {
		for y := -r; y <= r; y++ {
			if x*x+y*y <= r*r && x*x+y*y >= (r-5)*(r-5) {
				px, py := cx+x, cy+y
				if px >= 0 && px < img.Bounds().Dx() && py >= 0 && py < img.Bounds().Dy() {
					img.Set(px, py, c)
				}
			}
		}
	}
}

func addLine(img *image.RGBA, x1, y1, x2, y2 int, c color.Color) {
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	sx := 1
	if x1 > x2 {
		sx = -1
	}
	sy := 1
	if y1 > y2 {
		sy = -1
	}
	err := dx - dy
	for {
		if x1 >= 0 && x1 < img.Bounds().Dx() && y1 >= 0 && y1 < img.Bounds().Dy() {
			img.Set(x1, y1, c)
		}
		if x1 == x2 && y1 == y2 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
