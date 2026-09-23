package display

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

type statusRow struct {
	value string
	image *ebiten.Image
	start time.Time
}

func statusLines(status *Status) []string {
	if status == nil {
		return nil
	}
	if status.Lines != nil {
		return status.Lines
	}
	var lines []string
	for _, line := range []string{status.Title, status.Detail, status.Hint} {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func (g *windowGame) setStatus(status *Status, now time.Time) {
	if status != nil && status.FadeOut && (g.status == nil || !g.status.FadeOut) {
		g.statusFadeStart = now
		g.cover.fadeInNext = true
	}
	lines := statusLines(status)
	common := 0
	for common < len(lines) && common < len(g.statusRows) && lines[common] == g.statusRows[common].value {
		common++
	}
	for _, row := range g.statusRows[common:] {
		if row.image != nil {
			row.image.Deallocate()
		}
	}
	g.statusRows = g.statusRows[:common]
	for _, line := range lines[common:] {
		g.statusRows = append(g.statusRows, statusRow{value: line, start: now})
	}
	g.status = status
}

// Resizing invalidates raster images but preserves the per-line fade clocks.
func (g *windowGame) clearStatusImage() {
	for i := range g.statusRows {
		if g.statusRows[i].image != nil {
			g.statusRows[i].image.Deallocate()
			g.statusRows[i].image = nil
		}
	}
}

// Reserve space for the complete startup sequence, so existing lines never
// shift as new steps arrive. Scale down the instructions on smaller displays.
func statusLayout(text *textEngine, width, height int, scale float64, count int) (x, y, lineWidth, lineHeight int, textScale float64) {
	pad := max(1, int(math.Round(24*scale)))
	slots := max(9, count)
	textScale = min(scale, float64(max(1, height-2*pad))/(float64(slots)*text.size*1.7))
	lineHeight = max(1, int(math.Ceil(text.size*textScale*1.7)))
	lineWidth = max(1, min(width-2*pad, int(960*scale)))
	x = (width - lineWidth) / 2
	y = max(pad, (height-slots*lineHeight)/2)
	return
}

func (g *windowGame) drawStatus(screen *ebiten.Image) {
	alphaAll := 1.0
	settled := true
	if g.status.FadeOut {
		alphaAll = 0
		if g.cover.duration > 0 {
			alphaAll = 1 - g.cover.ease(clamp01(float64(g.now.Sub(g.statusFadeStart))/float64(g.cover.duration)))
		}
		settled = alphaAll <= 0
	}
	screen.Fill(color.NRGBA{R: uint8(18 * alphaAll), G: uint8(22 * alphaAll), B: uint8(30 * alphaAll), A: 255})
	x, y, width, lineHeight, scale := statusLayout(g.text, g.width, g.height, g.scale, len(g.statusRows))
	for i := range g.statusRows {
		row := &g.statusRows[i]
		if row.image == nil {
			pixels, _, err := g.text.raster(row.value, width, scale)
			if err != nil {
				g.err = err
				ebiten.ScheduleFrame()
				return
			}
			row.image = ebiten.NewImageFromImage(pixels)
		}
		alpha := 1.0
		if g.cover.duration > 0 {
			alpha = g.cover.ease(clamp01(float64(g.now.Sub(row.start)) / float64(g.cover.duration)))
		}
		if !g.status.FadeOut && alpha < 1 {
			settled = false
		}
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(x), float64(y+i*lineHeight))
		opts.ColorScale.ScaleAlpha(float32(alpha * alphaAll))
		screen.DrawImage(row.image, opts)
	}
	if settled {
		select {
		case g.status.Settled <- struct{}{}:
		default:
		}
	}
}

// Static rendering also supports visual checks without a graphics session.
func renderStatus(text *textEngine, status Status, width, height int, scale float64) (*image.NRGBA, error) {
	canvas := image.NewNRGBA(image.Rect(0, 0, max(1, width), max(1, height)))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.NRGBA{R: 18, G: 22, B: 30, A: 255}), image.Point{}, draw.Src)
	lines := statusLines(&status)
	x, y, lineWidth, lineHeight, textScale := statusLayout(text, width, height, scale, len(lines))
	for i, value := range lines {
		line, _, err := text.raster(value, lineWidth, textScale)
		if err != nil {
			return nil, err
		}
		draw.Draw(canvas, line.Bounds().Add(image.Pt(x, y+i*lineHeight)), line, line.Bounds().Min, draw.Over)
	}
	return canvas, nil
}
