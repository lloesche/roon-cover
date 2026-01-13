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
	Width        int
	Height       int
	Title        string
	Fullscreen   bool
	DisplayIndex int
	InfoCh       chan<- ScreenInfo
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

	// Log available displays at startup.
	numDisplays, err := sdl.GetNumVideoDisplays()
	if err == nil && numDisplays > 0 {
		for i := 0; i < numDisplays; i++ {
			name, nameErr := sdl.GetDisplayName(i)
			if nameErr != nil {
				name = "unknown"
			}
			b, bErr := sdl.GetDisplayBounds(i)
			if bErr != nil {
				continue
			}
			sdl.Log("display[%d]=%s bounds=%dx%d+%d+%d", i, name, b.W, b.H, b.X, b.Y)
		}
	}

	displayIndex := d.DisplayIndex
	if err != nil {
		sdl.LogWarn(sdl.LOG_CATEGORY_APPLICATION, "failed to enumerate SDL displays, defaulting to display 0: %v", err)
		displayIndex = 0
	} else if displayIndex < 0 || displayIndex >= numDisplays {
		sdl.LogWarn(sdl.LOG_CATEGORY_APPLICATION, "requested display %d is out of range (0..%d), falling back to 0", displayIndex, numDisplays-1)
		displayIndex = 0
	}
	sdl.Log("using display index %d", displayIndex)
	bounds, err := sdl.GetDisplayBounds(displayIndex)
	if err != nil {
		bounds = sdl.Rect{X: sdl.WINDOWPOS_CENTERED, Y: sdl.WINDOWPOS_CENTERED, W: int32(w), H: int32(h)}
	}

	flags := uint32(sdl.WINDOW_SHOWN)
	if !d.Fullscreen {
		flags |= sdl.WINDOW_RESIZABLE
	}

	win, err := sdl.CreateWindow(title, bounds.X, bounds.Y, int32(w), int32(h), flags)
	if err != nil {
		return err
	}
	defer win.Destroy()

	// Place the window on the chosen display before entering fullscreen.
	// (In windowed mode, this still picks the right screen.)
	if bounds.X != sdl.WINDOWPOS_CENTERED && bounds.Y != sdl.WINDOWPOS_CENTERED {
		win.SetPosition(bounds.X, bounds.Y)
	}

	if d.Fullscreen {
		// Fullscreen desktop preserves the display mode and avoids mode switches.
		if err := win.SetFullscreen(sdl.WINDOW_FULLSCREEN_DESKTOP); err != nil {
			return err
		}
	}

	ren, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		return err
	}
	defer ren.Destroy()

	reportOutputSize := func() {
		if d.InfoCh == nil {
			return
		}
		ww, wh, err := ren.GetOutputSize()
		if err != nil {
			return
		}
		select {
		case d.InfoCh <- ScreenInfo{DisplayIndex: displayIndex, RenderWidth: int(ww), RenderHeight: int(wh)}:
		default:
		}
	}

	// Report initial output size (important for choosing cover fetch size).
	reportOutputSize()

	var tex *sdl.Texture
	defer func() {
		if tex != nil {
			tex.Destroy()
		}
	}()

	var texW, texH int32

	render := func() error {
		ww, wh, err := ren.GetOutputSize()
		if err != nil {
			return err
		}

		// Clear background.
		_ = ren.SetDrawColor(0, 0, 0, 255)
		if err := ren.Clear(); err != nil {
			return err
		}

		if tex != nil {
			dst := fitRect(texW, texH, int32(ww), int32(wh))
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
					we := e.(*sdl.WindowEvent)
					// Keep output-size info up to date (window resize, display changes, etc.).
					switch we.Event {
					case sdl.WINDOWEVENT_RESIZED, sdl.WINDOWEVENT_SIZE_CHANGED, sdl.WINDOWEVENT_DISPLAY_CHANGED:
						reportOutputSize()
					}
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
