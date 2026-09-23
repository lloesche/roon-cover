package display

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

func (g *windowGame) clearStatusImage() {
	if g.statusImage != nil {
		g.statusImage.Deallocate()
		g.statusImage = nil
	}
}

func (g *windowGame) drawStatus(screen *ebiten.Image) {
	screen.Fill(color.NRGBA{R: 18, G: 22, B: 30, A: 255})
	if g.statusImage == nil {
		pixels, err := renderStatus(g.text, *g.status, g.width, g.height, g.scale)
		if err != nil {
			g.err = err
			ebiten.ScheduleFrame()
			return
		}
		g.statusImage = ebiten.NewImageFromImage(pixels)
	}
	screen.DrawImage(g.statusImage, nil)
}

// Rasterize only when the message or output size changes, using the same
// multilingual layout as song metadata. No overlay flags hide this screen.
func renderStatus(text *textEngine, status Status, width, height int, scale float64) (*image.NRGBA, error) {
	canvas := image.NewNRGBA(image.Rect(0, 0, max(1, width), max(1, height)))
	pad := int(math.Round(24 * scale))
	lineHeight := max(1, int(math.Ceil(text.size*scale*1.7)))
	y := max(pad, (height-3*lineHeight)/2)
	lines := []string{status.Title, status.Detail, status.Hint}
	for _, value := range lines {
		line, _, err := text.raster(value, max(1, width-2*pad), scale)
		if err != nil {
			return nil, err
		}
		x := (width - line.Bounds().Dx()) / 2
		draw.Draw(canvas, line.Bounds().Add(image.Pt(x, y)), line, line.Bounds().Min, draw.Over)
		y += lineHeight
	}
	return canvas, nil
}
