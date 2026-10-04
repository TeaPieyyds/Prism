package media

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

const asciiChars = " .:-=+*#%@"

// PixelToASCII converts a grayscale value (0-255) to an ASCII character
func PixelToASCII(gray uint8) string {
	idx := int(gray) * (len(asciiChars) - 1) / 255
	return string(asciiChars[idx])
}

// ImageToASCII converts any image.Image to ASCII art lines
func ImageToASCII(img image.Image, maxWidth int) ([]string, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	scale := 1.0
	if width > maxWidth {
		scale = float64(maxWidth) / float64(width)
	}
	newW := int(float64(width) * scale)
	newH := int(float64(height) * scale * 0.5)

	var lines []string
	for y := 0; y < newH; y++ {
		var line strings.Builder
		srcY := int(float64(y) / scale / 0.5)
		if srcY >= height {
			srcY = height - 1
		}
		for x := 0; x < newW; x++ {
			srcX := int(float64(x) / scale)
			if srcX >= width {
				srcX = width - 1
			}
			r, g, b, _ := img.At(srcX, srcY).RGBA()
			gray := uint8((299*uint32(r>>8) + 587*uint32(g>>8) + 114*uint32(b>>8)) / 1000)
			line.WriteString(PixelToASCII(gray))
		}
		lines = append(lines, line.String())
	}
	return lines, nil
}

// GenerateTitlerawCommands converts ASCII frame lines to Minecraft titleraw commands
func GenerateTitlerawCommands(lines []string, target string, mode string) []string {
	content := strings.Join(lines, "\\n")
	cmd := fmt.Sprintf(
		`titleraw %s %s {"rawtext":[{"text":"%s"}]}`,
		target, mode, content,
	)
	return []string{cmd}
}
