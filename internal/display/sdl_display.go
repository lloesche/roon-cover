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
	"math"
	"strings"
	"time"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

type SDLDisplay struct {
	Width        int
	Height       int
	Title        string
	Fullscreen   bool
	DisplayIndex int
	InfoCh       chan<- ScreenInfo
	FadeMS       int
	Ease         string

	ShowTitle  bool
	ShowArtist bool
	ShowAlbum  bool

	FontPath string
	FontSize int
}

func (d *SDLDisplay) Run(ctx context.Context, updates <-chan Update) error {
	fadeMS := d.FadeMS
	if fadeMS < 0 {
		return errors.New("display: fade-ms must be >= 0")
	}
	fadeDur := time.Duration(fadeMS) * time.Millisecond

	easeName := strings.TrimSpace(d.Ease)
	if easeName == "" {
		easeName = "in-out-sine"
	}
	ease, err := EasingByName(easeName)
	if err != nil {
		return err
	}

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

	if err := ttf.Init(); err != nil {
		return err
	}
	defer ttf.Quit()

	fontPath, err := resolveFontPath(d.FontPath)
	if err != nil {
		// Only require a font if overlay is enabled.
		if d.ShowTitle || d.ShowArtist || d.ShowAlbum {
			return err
		}
		fontPath = ""
	}
	fontSize := d.FontSize
	if fontSize <= 0 {
		fontSize = 28
	}

	var font *ttf.Font
	if fontPath != "" && (d.ShowTitle || d.ShowArtist || d.ShowAlbum) {
		f, err := ttf.OpenFont(fontPath, fontSize)
		if err != nil {
			return err
		}
		font = f
		defer font.Close()
	}

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

	// Cursor behavior: hide only when running fullscreen and the window is focused.
	// Always ensure we restore cursor visibility on exit.
	defer sdl.ShowCursor(sdl.ENABLE)
	if d.Fullscreen {
		sdl.ShowCursor(sdl.DISABLE)
	} else {
		sdl.ShowCursor(sdl.ENABLE)
	}

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

	var currTex *sdl.Texture
	var prevTex *sdl.Texture
	defer func() {
		if currTex != nil {
			currTex.Destroy()
		}
		if prevTex != nil {
			prevTex.Destroy()
		}
	}()

	var currW, currH int32
	var prevW, prevH int32

	// Text overlay state (optional).
	var currTextTex *sdl.Texture
	var prevTextTex *sdl.Texture
	var currTextW, currTextH int32
	var prevTextW, prevTextH int32
	var currTextLines []string
	defer func() {
		if currTextTex != nil {
			currTextTex.Destroy()
		}
		if prevTextTex != nil {
			prevTextTex.Destroy()
		}
	}()

	var coverFading bool
	var coverFadeStart time.Time

	var textFading bool
	var textFadeStart time.Time

	alphaU8 := func(v float64) uint8 {
		if v <= 0 {
			return 0
		}
		if v >= 1 {
			return 255
		}
		return uint8(math.Round(v * 255))
	}

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

		drawTex := func(tex *sdl.Texture, tw, th int32) error {
			if tex == nil {
				return nil
			}
			dst := fitRect(tw, th, int32(ww), int32(wh))
			return ren.Copy(tex, nil, &dst)
		}

		// Draw prev first, then curr on top (during fades).
		if prevTex != nil {
			if err := drawTex(prevTex, prevW, prevH); err != nil {
				return err
			}
		}
		if currTex != nil {
			if err := drawTex(currTex, currW, currH); err != nil {
				return err
			}
		}

		// Draw text overlay (with matching fade alphas).
		drawText := func(tex *sdl.Texture, tw, th int32, alpha uint8) error {
			if tex == nil || tw <= 0 || th <= 0 {
				return nil
			}
			_ = tex.SetAlphaMod(alpha)
			pad := int32(24)
			x := pad
			y := int32(wh) - pad - th
			dst := sdl.Rect{X: x, Y: y, W: tw, H: th}

			return ren.Copy(tex, nil, &dst)
		}

		// Text fade can be driven either by a cover fade (when both change) or by a text-only fade.
		// Prefer the cover fade when active so text stays in sync with cover transitions.
		switch {
		case coverFading && fadeDur > 0 && prevTextTex != nil && currTextTex != nil:
			t := float64(time.Since(coverFadeStart)) / float64(fadeDur)
			e := clamp01(ease(t))
			aPrev := alphaU8(1 - e)
			aCurr := alphaU8(e)
			if err := drawText(prevTextTex, prevTextW, prevTextH, aPrev); err != nil {
				return err
			}
			if err := drawText(currTextTex, currTextW, currTextH, aCurr); err != nil {
				return err
			}
		case textFading && fadeDur > 0 && prevTextTex != nil && currTextTex != nil:
			t := float64(time.Since(textFadeStart)) / float64(fadeDur)
			e := clamp01(ease(t))
			aPrev := alphaU8(1 - e)
			aCurr := alphaU8(e)
			if err := drawText(prevTextTex, prevTextW, prevTextH, aPrev); err != nil {
				return err
			}
			if err := drawText(currTextTex, currTextW, currTextH, aCurr); err != nil {
				return err
			}
		default:
			if err := drawText(currTextTex, currTextW, currTextH, 255); err != nil {
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

			textChanged := false

			// Update text overlay (if enabled) whenever metadata changes.
			if font != nil && (d.ShowTitle || d.ShowArtist || d.ShowAlbum) {
				lines := buildOverlayLines(u.NowPlaying, d.ShowTitle, d.ShowArtist, d.ShowAlbum)
				sameText := sameLines(lines, currTextLines)

				// If there's no text to show, clear any existing overlay.
				if len(lines) == 0 {
					if currTextTex != nil {
						currTextTex.Destroy()
						currTextTex = nil
					}
					if prevTextTex != nil {
						prevTextTex.Destroy()
						prevTextTex = nil
					}
					currTextW, currTextH = 0, 0
					prevTextW, prevTextH = 0, 0
					currTextLines = nil
					textChanged = true
					textFading = false
				} else if !sameText {
					newTextTex, newTW, newTH, err := renderTextBlock(ren, font, lines)
					if err == nil {
						immediate := u.NoFade || fadeDur <= 0 || currTextTex == nil
						if immediate {
							if prevTextTex != nil {
								prevTextTex.Destroy()
								prevTextTex = nil
							}
							if currTextTex != nil {
								currTextTex.Destroy()
								currTextTex = nil
							}
							currTextTex, currTextW, currTextH = newTextTex, newTW, newTH
							currTextLines = lines
							textFading = false
						} else {
							if prevTextTex != nil {
								prevTextTex.Destroy()
								prevTextTex = nil
							}
							prevTextTex, prevTextW, prevTextH = currTextTex, currTextW, currTextH

							currTextTex, currTextW, currTextH = newTextTex, newTW, newTH
							currTextLines = lines

							// Text-only transition: fade between the previous and current text.
							textFadeStart = time.Now()
							textFading = true
						}
						textChanged = true
					}
				}
			}

			if len(u.CoverImage) > 0 {
				img, _, err := image.Decode(bytes.NewReader(u.CoverImage))
				if err != nil {
					// Ignore bad image frames; keep last image.
					break
				}

				rgba := image.NewRGBA(img.Bounds())
				draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)

				// Build a new texture for this frame.
				newW := int32(rgba.Bounds().Dx())
				newH := int32(rgba.Bounds().Dy())
				newTex, err := ren.CreateTexture(sdl.PIXELFORMAT_ABGR8888, sdl.TEXTUREACCESS_STATIC, newW, newH)
				if err != nil {
					return err
				}
				_ = newTex.SetBlendMode(sdl.BLENDMODE_BLEND)

				pitch := rgba.Stride
				if len(rgba.Pix) == 0 {
					newTex.Destroy()
					return errors.New("decoded image has no pixel data")
				}
				if err := newTex.Update(nil, unsafe.Pointer(&rgba.Pix[0]), pitch); err != nil {
					newTex.Destroy()
					return err
				}

				// Install texture (optionally crossfading).
				if u.NoFade || fadeDur <= 0 || currTex == nil {
					if prevTex != nil {
						prevTex.Destroy()
						prevTex = nil
					}
					if currTex != nil {
						currTex.Destroy()
						currTex = nil
					}
					currTex = newTex
					currW, currH = newW, newH
					_ = currTex.SetAlphaMod(255)
					coverFading = false
				} else {
					// If a previous fade is in-flight, discard the older prevTex to avoid leaks.
					if prevTex != nil {
						prevTex.Destroy()
						prevTex = nil
					}
					prevTex, prevW, prevH = currTex, currW, currH
					currTex, currW, currH = newTex, newW, newH

					// Reset alpha mods for a clean crossfade.
					_ = prevTex.SetAlphaMod(255)
					_ = currTex.SetAlphaMod(0)

					coverFadeStart = time.Now()
					coverFading = true
				}

				if err := render(); err != nil {
					return err
				}
			}

			// Text-only update (same cover, new metadata) should repaint immediately.
			if textChanged && len(u.CoverImage) == 0 {
				if err := render(); err != nil {
					return err
				}
			}

		case <-eventsTick.C:
			// Progress fade (if active).
			if coverFading && fadeDur > 0 && prevTex != nil && currTex != nil {
				t := float64(time.Since(coverFadeStart)) / float64(fadeDur)
				if t >= 1 {
					coverFading = false
					prevTex.Destroy()
					prevTex = nil
					_ = currTex.SetAlphaMod(255)
					if err := render(); err != nil {
						return err
					}
				} else {
					e := clamp01(ease(t))
					_ = prevTex.SetAlphaMod(alphaU8(1 - e))
					_ = currTex.SetAlphaMod(alphaU8(e))
					if err := render(); err != nil {
						return err
					}
				}
			}

			// Progress text-only fade (if active).
			if textFading && fadeDur > 0 && prevTextTex != nil && currTextTex != nil {
				t := float64(time.Since(textFadeStart)) / float64(fadeDur)
				if t >= 1 {
					textFading = false
					prevTextTex.Destroy()
					prevTextTex = nil
					if err := render(); err != nil {
						return err
					}
				} else {
					// Keep redrawing while fading (alphas are computed in render()).
					if err := render(); err != nil {
						return err
					}
				}
			}

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
					case sdl.WINDOWEVENT_FOCUS_GAINED:
						if d.Fullscreen {
							sdl.ShowCursor(sdl.DISABLE)
						} else {
							sdl.ShowCursor(sdl.ENABLE)
						}
					case sdl.WINDOWEVENT_FOCUS_LOST:
						// When not focused, allow cursor to be visible.
						sdl.ShowCursor(sdl.ENABLE)
					case sdl.WINDOWEVENT_RESIZED, sdl.WINDOWEVENT_SIZE_CHANGED, sdl.WINDOWEVENT_DISPLAY_CHANGED:
						reportOutputSize()
						// Ensure the current texture is repainted at the new size.
						if err := render(); err != nil {
							return err
						}
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
