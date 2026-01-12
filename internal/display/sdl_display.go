//go:build sdl

package display

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"time"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

type SDLDisplay struct {
	Width  int
	Height int
	Title  string
}

func (d *SDLDisplay) Run(ctx context.Context, updates <-chan Update) error {
	w := d.Width
	h := d.Height
	if w <= 0 {
		w = 800
	}
	if h <= 0 {
		h = 800
	}
	title := d.Title
	if title == "" {
		title = "roon-cover"
	}

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		return err
	}
	defer sdl.Quit()

	win, err := sdl.CreateWindow(title, sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED, int32(w), int32(h), sdl.WINDOW_SHOWN)
	if err != nil {
		return err
	}
	defer win.Destroy()

	ren, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		return err
	}
	defer ren.Destroy()

	var tex *sdl.Texture
	defer func() {
		if tex != nil {
			tex.Destroy()
		}
	}()

	var texW, texH int32

	render := func() error {
		// Clear background.
		_ = ren.SetDrawColor(0, 0, 0, 255)
		if err := ren.Clear(); err != nil {
			return err
		}

		if tex != nil {
			ww, wh := int32(w), int32(h)
			dst := fitRect(texW, texH, ww, wh)
			if err := ren.Copy(tex, nil, &dst); err != nil {
				return err
			}
		}

		ren.Present()
		return nil
	}

	// Initial paint.
	if err := render(); err != nil {
		return err
	}

	eventsTick := time.NewTicker(16 * time.Millisecond)
	defer eventsTick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case u, ok := <-updates:
			if !ok {
				return nil
			}
			if len(u.CoverImage) > 0 {
				img, _, err := image.Decode(bytes.NewReader(u.CoverImage))
				if err != nil {
					// Ignore bad image frames; keep last image.
					break
				}

				rgba := image.NewRGBA(img.Bounds())
				draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)

				if tex != nil {
					tex.Destroy()
					tex = nil
				}

				texW = int32(rgba.Bounds().Dx())
				texH = int32(rgba.Bounds().Dy())

				tex, err = ren.CreateTexture(sdl.PIXELFORMAT_ABGR8888, sdl.TEXTUREACCESS_STATIC, texW, texH)
				if err != nil {
					return err
				}
				_ = tex.SetBlendMode(sdl.BLENDMODE_BLEND)

				pitch := rgba.Stride
				if len(rgba.Pix) == 0 {
					return errors.New("decoded image has no pixel data")
				}
				if err := tex.Update(nil, unsafe.Pointer(&rgba.Pix[0]), pitch); err != nil {
					return err
				}

				if err := render(); err != nil {
					return err
				}
			}

			// TODO: overlay text (title/artist/album) via SDL_ttf or bitmap font.
			// u.NowPlaying already contains the fields we need.

		case <-eventsTick.C:
			for {
				e := sdl.PollEvent()
				if e == nil {
					break
				}
				switch e.(type) {
				case *sdl.QuitEvent:
					return nil
				case *sdl.WindowEvent:
					// Repaint on expose/resize (future).
				}
			}
		}
	}
}

func fitRect(srcW, srcH, dstW, dstH int32) sdl.Rect {
	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 {
		return sdl.Rect{X: 0, Y: 0, W: dstW, H: dstH}
	}

	// Letterbox fit.
	scaleW := float64(dstW) / float64(srcW)
	scaleH := float64(dstH) / float64(srcH)
	scale := scaleW
	if scaleH < scaleW {
		scale = scaleH
	}

	w := int32(float64(srcW) * scale)
	h := int32(float64(srcH) * scale)
	x := (dstW - w) / 2
	y := (dstH - h) / 2
	return sdl.Rect{X: x, Y: y, W: w, H: h}
}
