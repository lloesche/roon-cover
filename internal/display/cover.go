package display

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image/color"
	"math"
	"time"
)

// Each cover is an opaque full-output canvas, so transitions also blend
// letterboxing correctly. Interruptions snapshot the currently visible blend.
type coverLayer struct {
	current, previous *ebiten.Image
	asset             *Artwork
	width, height     int
	start             time.Time
	duration          time.Duration
	ease              func(float64) float64
}

func (c *coverLayer) close() {
	if c.current != nil {
		c.current.Deallocate()
	}
	if c.previous != nil {
		c.previous.Deallocate()
	}
	c.current = nil
	c.previous = nil
	c.asset = nil
}
func (c *coverLayer) active(now time.Time) bool {
	return c.previous != nil && now.Sub(c.start) < c.duration
}
func (c *coverLayer) draw(dst *ebiten.Image, now time.Time) {
	if c.previous != nil && !c.active(now) {
		c.previous.Deallocate()
		c.previous = nil
	}
	if c.previous != nil {
		dst.DrawImage(c.previous, nil)
	}
	if c.current == nil {
		return
	}
	opts := &ebiten.DrawImageOptions{}
	if c.active(now) {
		opts.ColorScale.ScaleAlpha(float32(clamp01(c.ease(float64(now.Sub(c.start)) / float64(c.duration)))))
	}
	dst.DrawImage(c.current, opts)
}
func (c *coverLayer) set(asset *Artwork, noFade bool, now time.Time) {
	if asset == nil || asset.Pixels == nil {
		c.close()
		return
	}
	if asset == c.asset {
		return
	}
	canvas := ebiten.NewImage(max(1, c.width), max(1, c.height))
	canvas.Fill(color.Black)
	p := asset.Pixels
	texture := ebiten.NewImageFromImage(p)
	scale := math.Min(float64(c.width)/float64(p.Bounds().Dx()), float64(c.height)/float64(p.Bounds().Dy()))
	opts := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate((float64(c.width)-float64(p.Bounds().Dx())*scale)/2, (float64(c.height)-float64(p.Bounds().Dy())*scale)/2)
	canvas.DrawImage(texture, opts)
	texture.Deallocate()
	var previous *ebiten.Image
	if !noFade && c.duration > 0 && c.current != nil {
		previous = ebiten.NewImage(max(1, c.width), max(1, c.height))
		previous.Fill(color.Black)
		c.draw(previous, now)
	}
	c.close()
	c.current = canvas
	c.previous = previous
	c.asset = asset
	c.start = now
}
func (c *coverLayer) resize(w, h int, now time.Time) {
	asset := c.asset
	c.width = w
	c.height = h
	c.asset = nil
	c.set(asset, true, now)
}
