// Command mkpreview generates preview and thumbnail images showing a
// simplified NetHack dungeon scene rendered in classic terminal style.
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// A small NetHack-like dungeon scene.
var scene = []string{
	"                                                ",
	"       ------+------                            ",
	"       |.........$.| ####                       ",
	"       |..d........+##  #       --------        ",
	"       |...........|   ###      |......|        ",
	"       |..@........|     #      |..k...|        ",
	"       |.......[...|     #######+......|        ",
	"       |...........|            |...D.%.|        ",
	"       ----------+--            -----+--        ",
	"                 #                   #          ",
	"                 #                   #          ",
	"           ------+------       ------+------    ",
	"           |...........|       |...........|    ",
	"           |....)......|       |.......$$. |    ",
	"           |...........|       |...........|    ",
	"           -------------       -------------    ",
	"                                                ",
}

func charColor(ch byte) color.RGBA {
	switch ch {
	case '@':
		return color.RGBA{255, 255, 255, 255}
	case 'd':
		return color.RGBA{180, 130, 50, 255}
	case 'D':
		return color.RGBA{255, 50, 50, 255}
	case 'k':
		return color.RGBA{50, 200, 50, 255}
	case '$':
		return color.RGBA{255, 255, 0, 255}
	case '[':
		return color.RGBA{100, 100, 255, 255}
	case ')':
		return color.RGBA{100, 100, 255, 255}
	case '%':
		return color.RGBA{180, 50, 50, 255}
	case '#':
		return color.RGBA{140, 140, 140, 255}
	case '.':
		return color.RGBA{140, 140, 140, 255}
	case '-', '|', '+':
		return color.RGBA{180, 180, 180, 255}
	default:
		return color.RGBA{0, 0, 0, 255}
	}
}

// render draws the scene at 1x and returns the image.
func render() *image.RGBA {
	face := basicfont.Face7x13
	metrics := face.Metrics()
	cellH := metrics.Height.Ceil()
	cellW := 7

	cols := len(scene[0])
	rows := len(scene)

	srcW := cols * cellW
	srcH := rows * cellH
	img := image.NewRGBA(image.Rect(0, 0, srcW, srcH))

	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			img.Set(x, y, color.Black)
		}
	}

	for row, line := range scene {
		for col := 0; col < len(line); col++ {
			ch := line[col]
			if ch == ' ' {
				continue
			}
			d := &font.Drawer{
				Dst:  img,
				Src:  image.NewUniform(charColor(ch)),
				Face: face,
				Dot:  fixed.P(col*cellW, row*cellH+metrics.Ascent.Ceil()),
			}
			d.DrawString(string(ch))
		}
	}
	return img
}

// nearestScale scales src by an integer factor using nearest-neighbor.
func nearestScale(src *image.RGBA, scale int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*scale, b.Dy()*scale))
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			dst.Set(x, y, src.At(x/scale, y/scale))
		}
	}
	return dst
}

// resizeTo scales src to exactly dstW x dstH using nearest-neighbor.
func resizeTo(src *image.RGBA, dstW, dstH int) *image.RGBA {
	b := src.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			srcX := x * srcW / dstW
			srcY := y * srcH / dstH
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func writePNG(name string, img image.Image) {
	f, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		panic(err)
	}
	f.Close()
}

// ensureAlpha makes sure every pixel has an explicit alpha of 255,
// preventing the PNG encoder from stripping the alpha channel.
func ensureAlpha(img *image.RGBA) {
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
}

func main() {
	src := render()
	ensureAlpha(src)

	// preview.png — 3x scaled for the live preview pane in System Settings
	writePNG("preview.png", nearestScale(src, 3))

	// thumbnail.png — 90x58 for the screensaver selection grid (1x)
	thumb1x := resizeTo(src, 90, 58)
	ensureAlpha(thumb1x)
	writePNG("thumbnail.png", thumb1x)

	// thumbnail@2x.png — 180x116 for the screensaver selection grid (Retina)
	thumb2x := resizeTo(src, 180, 116)
	ensureAlpha(thumb2x)
	writePNG("thumbnail@2x.png", thumb2x)
}
