//go:build sdl

package display

import (
	"github.com/veandco/go-sdl2/sdl"
	"time"
	"unsafe"
)

// coverLayer owns all cover textures. Both sides of a transition are opaque
// full-output canvases, including letterboxing. Interrupted transitions snapshot
// the current composition instead of jumping to an unfinished target.
type coverLayer struct {
	ren                      *sdl.Renderer
	image, current, previous *sdl.Texture
	width, height            int32
	start                    time.Time
	duration                 time.Duration
	ease                     func(float64) float64
	asset                    *Artwork
}

func (c *coverLayer) close() {
	for _, t := range []*sdl.Texture{c.image, c.current, c.previous} {
		if t != nil {
			t.Destroy()
		}
	}
	c.image = nil
	c.current = nil
	c.previous = nil
	c.asset = nil
}
func (c *coverLayer) active(now time.Time) bool {
	return c.previous != nil && now.Sub(c.start) < c.duration
}
func (c *coverLayer) draw(now time.Time) error {
	if c.previous != nil && !c.active(now) {
		c.previous.Destroy()
		c.previous = nil
	}
	if c.previous != nil {
		if err := c.ren.Copy(c.previous, nil, nil); err != nil {
			return err
		}
	}
	if c.current == nil {
		return nil
	}
	a := uint8(255)
	if c.active(now) {
		_, a, _ = coverFadeAlphas(float64(now.Sub(c.start))/float64(c.duration), c.ease)
	}
	if err := c.current.SetAlphaMod(a); err != nil {
		return err
	}
	return c.ren.Copy(c.current, nil, nil)
}
func (c *coverLayer) canvas(w, h int32, paint func() error) (*sdl.Texture, error) {
	t, err := c.ren.CreateTexture(sdl.PIXELFORMAT_RGBA8888, sdl.TEXTUREACCESS_TARGET, w, h)
	if err != nil {
		return nil, err
	}
	old := c.ren.GetRenderTarget()
	if err = c.ren.SetRenderTarget(t); err != nil {
		t.Destroy()
		return nil, err
	}
	defer c.ren.SetRenderTarget(old)
	if err = c.ren.SetDrawColor(0, 0, 0, 255); err == nil {
		err = c.ren.Clear()
	}
	if err == nil {
		err = paint()
	}
	if err != nil {
		t.Destroy()
		return nil, err
	}
	if err = t.SetBlendMode(sdl.BLENDMODE_BLEND); err != nil {
		t.Destroy()
		return nil, err
	}
	return t, nil
}
func (c *coverLayer) set(asset *Artwork, noFade bool, now time.Time) error {
	if asset == nil {
		c.close()
		return nil
	}
	if asset == c.asset {
		return nil
	}
	w, h, err := c.ren.GetOutputSize()
	if err != nil {
		return err
	}
	p := asset.Pixels
	t, err := c.ren.CreateTexture(sdl.PIXELFORMAT_ABGR8888, sdl.TEXTUREACCESS_STATIC, int32(p.Bounds().Dx()), int32(p.Bounds().Dy()))
	if err != nil {
		return err
	}
	if err = t.SetBlendMode(sdl.BLENDMODE_BLEND); err == nil {
		err = t.Update(nil, unsafe.Pointer(&p.Pix[0]), p.Stride)
	}
	if err != nil {
		t.Destroy()
		return err
	}
	canvas, err := c.canvas(int32(w), int32(h), func() error {
		dst := fitRect(int32(p.Bounds().Dx()), int32(p.Bounds().Dy()), int32(w), int32(h))
		return c.ren.Copy(t, nil, &dst)
	})
	if err != nil {
		t.Destroy()
		return err
	}
	var previous *sdl.Texture
	if !noFade && c.duration > 0 && c.current != nil {
		previous, err = c.canvas(int32(w), int32(h), func() error { return c.draw(now) })
		if err != nil {
			canvas.Destroy()
			t.Destroy()
			return err
		}
	}
	c.close()
	c.image = t
	c.current = canvas
	c.previous = previous
	c.asset = asset
	c.width = int32(w)
	c.height = int32(h)
	c.start = now
	return nil
}
func (c *coverLayer) resize(now time.Time) error {
	a := c.asset
	if a == nil {
		return nil
	}
	c.asset = nil
	return c.set(a, true, now)
}
