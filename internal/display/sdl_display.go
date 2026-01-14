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
	EventCh      chan<- Event
	FadeMS       int
	Ease         string

	ShowTitle  bool
	ShowArtist bool
	ShowAlbum  bool
	ShowZone   bool

	FontPath   string
	FontSize   int
	FontFadeMS int
}

func (d *SDLDisplay) Run(ctx context.Context, updates <-chan Update) error {
	fadeMS := d.FadeMS
	if fadeMS < 0 {
		return errors.New("display: fade-ms must be >= 0")
	}
	fadeDur := time.Duration(fadeMS) * time.Millisecond

	fontFadeMS := d.FontFadeMS
	if fontFadeMS < 0 {
		return errors.New("display: font-fade-ms must be >= 0")
	}
	fontFadeDur := time.Duration(fontFadeMS) * time.Millisecond

	// How long the transient zone-name overlay stays visible before fading out.
	const zoneOverlayVisibleFor = 3 * time.Second

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
		if d.ShowTitle || d.ShowArtist || d.ShowAlbum || d.ShowZone {
			return err
		}
		fontPath = ""
	}
	fontSize := d.FontSize
	if fontSize <= 0 {
		fontSize = 28
	}

	var fonts []*ttf.Font
	if d.ShowTitle || d.ShowArtist || d.ShowAlbum || d.ShowZone {
		// Build a small font stack: primary first, then fallbacks.
		// This allows rendering Unicode like U+2010 (‐) without normalizing text.
		candidatePaths := make([]string, 0, 8)
		seen := map[string]struct{}{}
		addPath := func(p string) {
			p = strings.TrimSpace(p)
			if p == "" {
				return
			}
			if _, ok := seen[p]; ok {
				return
			}
			seen[p] = struct{}{}
			candidatePaths = append(candidatePaths, p)
		}

		if fontPath != "" {
			addPath(fontPath)
		}
		for _, p := range defaultFontCandidates() {
			addPath(p)
		}

		for _, p := range candidatePaths {
			f, err := ttf.OpenFont(p, fontSize)
			if err != nil {
				continue
			}
			fonts = append(fonts, f)
			// Keep the stack small.
			if len(fonts) >= 5 {
				break
			}
		}
		defer func() {
			for _, f := range fonts {
				if f != nil {
					f.Close()
				}
			}
		}()

		if len(fonts) == 0 {
			return errors.New("no usable fonts available; specify one via --font")
		}
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

	var coverFading bool
	var coverFadeStart time.Time

	type textLine struct {
		key string

		currStr string
		currTex *sdl.Texture
		currW   int32
		currH   int32

		pendingStr string
		pendingTex *sdl.Texture
		pendingW   int32
		pendingH   int32

		phase     textPhase
		fadeStart time.Time

		// For transient overlays like the zone name: if non-zero, start fading out after this time.
		hideAt time.Time
	}

	clearLine := func(l *textLine) {
		if l.currTex != nil {
			l.currTex.Destroy()
			l.currTex = nil
		}
		if l.pendingTex != nil {
			l.pendingTex.Destroy()
			l.pendingTex = nil
		}
		l.currStr = ""
		l.pendingStr = ""
		l.currW, l.currH, l.pendingW, l.pendingH = 0, 0, 0, 0
		l.phase = 0
		l.hideAt = time.Time{}
	}

	titleLine := textLine{key: "title"}
	artistLine := textLine{key: "artist"}
	albumLine := textLine{key: "album"}
	zoneLine := textLine{key: "zone"}
	defer func() {
		clearLine(&titleLine)
		clearLine(&artistLine)
		clearLine(&albumLine)
		clearLine(&zoneLine)
	}()

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

		// drawLine draws a line texture at (x,y). We fade text via alpha modulation.
		// A small gamma curve helps keep the fade perceptually smooth and avoids "black ghost" artifacts.
		drawLine := func(tex *sdl.Texture, tw, th int32, x, y int32, intensity float64) error {
			if tex == nil || tw <= 0 || th <= 0 {
				return nil
			}
			intensity = clamp01(intensity)
			_ = tex.SetBlendMode(sdl.BLENDMODE_BLEND)

			// Perceptual curve: make very-low alpha spend less time looking jagged.
			// (Also ensures intensity=0 actually draws nothing.)
			i := math.Sqrt(intensity)

			// Shadow: draw the same glyph mask in black behind the text, fading with intensity.
			shadowOffsets := []sdl.Point{
				{X: 2, Y: 2},
				{X: 2, Y: 3},
				{X: 3, Y: 2},
				{X: 3, Y: 3},
			}
			shadowAlpha := uint8(math.Round(160 * i))
			_ = tex.SetColorMod(0, 0, 0)
			_ = tex.SetAlphaMod(shadowAlpha)
			for _, off := range shadowOffsets {
				dst := sdl.Rect{X: x + int32(off.X), Y: y + int32(off.Y), W: tw, H: th}
				if err := ren.Copy(tex, nil, &dst); err != nil {
					return err
				}
			}

			// Main text: fade via alpha.
			mainAlpha := uint8(math.Round(255 * i))
			_ = tex.SetColorMod(255, 255, 255)
			_ = tex.SetAlphaMod(mainAlpha)
			dst := sdl.Rect{X: x, Y: y, W: tw, H: th}
			return ren.Copy(tex, nil, &dst)
		}

		// Zone overlay (top-left, transient).
		if d.ShowZone && zoneLine.currTex != nil && strings.TrimSpace(zoneLine.currStr) != "" {
			alpha := 1.0
			if zoneLine.phase != 0 && fontFadeDur > 0 {
				t := float64(time.Since(zoneLine.fadeStart)) / float64(fontFadeDur)
				alpha = textIntensity(zoneLine.phase, t, ease)
			}
			pad := int32(24)
			if err := drawLine(zoneLine.currTex, zoneLine.currW, zoneLine.currH, pad, pad, alpha); err != nil {
				return err
			}
		}

		// Draw per-line overlays. Only lines that actually changed are faded.
		pad := int32(24)
		lineGap := int32(6)
		x := pad

		lines := make([]*textLine, 0, 3)
		if d.ShowTitle && titleLine.currTex != nil {
			lines = append(lines, &titleLine)
		}
		if d.ShowArtist && artistLine.currTex != nil {
			lines = append(lines, &artistLine)
		}
		if d.ShowAlbum && albumLine.currTex != nil {
			lines = append(lines, &albumLine)
		}

		totalH := int32(0)
		for _, ln := range lines {
			h := ln.currH
			totalH += h
		}
		if len(lines) > 1 {
			totalH += int32(len(lines)-1) * lineGap
		}
		y := int32(wh) - pad - totalH

		for _, ln := range lines {
			h := ln.currH
			alpha := 1.0
			if ln.phase != 0 && fontFadeDur > 0 {
				t := float64(time.Since(ln.fadeStart)) / float64(fontFadeDur)
				alpha = textIntensity(ln.phase, t, ease)
			}
			if err := drawLine(ln.currTex, ln.currW, ln.currH, x, y, alpha); err != nil {
				return err
			}

			y += h + lineGap
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

			coverUpdatedThisUpdate := false
			textUpdatedThisUpdate := false

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
				coverUpdatedThisUpdate = true
			}

			// Update per-line text overlays (if enabled).
			if len(fonts) > 0 && (d.ShowTitle || d.ShowArtist || d.ShowAlbum || d.ShowZone) {
				var title, artist, album string
				if u.NowPlaying != nil {
					title = strings.TrimSpace(u.NowPlaying.Title)
					artist = strings.TrimSpace(u.NowPlaying.Artist)
					album = strings.TrimSpace(u.NowPlaying.Album)
				}

				updateLine := func(ln *textLine, next string) error {
					// Disabled or empty -> clear.
					if strings.TrimSpace(next) == "" {
						if ln.currStr != "" || ln.currTex != nil || ln.pendingTex != nil {
							clearLine(ln)
							textUpdatedThisUpdate = true
						}
						return nil
					}
					if next == ln.currStr {
						return nil
					}

					newTex, newW, newH, err := renderTextLine(ren, fonts, next)
					if err != nil {
						return err
					}
					if newTex == nil {
						clearLine(ln)
						ln.currStr = ""
						return nil
					}

					immediate := u.NoFade || fontFadeDur <= 0 || ln.currTex == nil
					if immediate {
						if ln.currTex != nil {
							ln.currTex.Destroy()
							ln.currTex = nil
						}
						if ln.pendingTex != nil {
							ln.pendingTex.Destroy()
							ln.pendingTex = nil
						}
						ln.currTex, ln.currW, ln.currH = newTex, newW, newH
						ln.currStr = next
						ln.pendingStr = ""
						ln.phase = 0
						if ln.key == "zone" {
							ln.hideAt = time.Now().Add(zoneOverlayVisibleFor)
						}
						textUpdatedThisUpdate = true
						return nil
					}

					// Fade-out-in: hold current, fade out; then swap to pending; then fade in.
					if ln.pendingTex != nil {
						ln.pendingTex.Destroy()
						ln.pendingTex = nil
					}
					ln.pendingTex, ln.pendingW, ln.pendingH = newTex, newW, newH
					ln.pendingStr = next

					ln.phase = textPhaseFadeOut
					ln.fadeStart = time.Now()
					if ln.key == "zone" {
						// Restart the visibility window on zone changes.
						ln.hideAt = time.Now().Add(zoneOverlayVisibleFor)
					}
					textUpdatedThisUpdate = true
					return nil
				}

				if d.ShowZone {
					if err := updateLine(&zoneLine, strings.TrimSpace(u.Zone)); err != nil {
						return err
					}
				} else {
					clearLine(&zoneLine)
				}

				if d.ShowTitle {
					if err := updateLine(&titleLine, title); err != nil {
						return err
					}
				} else {
					clearLine(&titleLine)
				}
				if d.ShowArtist {
					if err := updateLine(&artistLine, artist); err != nil {
						return err
					}
				} else {
					clearLine(&artistLine)
				}
				if d.ShowAlbum {
					if err := updateLine(&albumLine, album); err != nil {
						return err
					}
				} else {
					clearLine(&albumLine)
				}

				// (render happens once at the end of the update)
			}

			// Ensure the very first update renders both cover + text (startup text bug fix).
			if coverUpdatedThisUpdate || textUpdatedThisUpdate {
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
					pA, cA, _ := coverFadeAlphas(t, ease)
					_ = prevTex.SetAlphaMod(pA)
					_ = currTex.SetAlphaMod(cA)
					if err := render(); err != nil {
						return err
					}
				}
			}

			// Auto-hide zone overlay after a short delay.
			zoneNeedsHide := false
			if d.ShowZone && zoneLine.currTex != nil && strings.TrimSpace(zoneLine.currStr) != "" && zoneLine.phase == 0 && !zoneLine.hideAt.IsZero() && time.Now().After(zoneLine.hideAt) && fontFadeDur > 0 {
				// Fade out to "empty" (no fade-in).
				if zoneLine.pendingTex != nil {
					zoneLine.pendingTex.Destroy()
					zoneLine.pendingTex = nil
				}
				zoneLine.pendingStr = ""
				zoneLine.pendingW, zoneLine.pendingH = 0, 0
				zoneLine.phase = textPhaseFadeOut
				zoneLine.fadeStart = time.Now()
				zoneNeedsHide = true
			}

			progressLine := func(ln *textLine) bool {
				if ln.phase == 0 || fontFadeDur <= 0 {
					return false
				}
				t := float64(time.Since(ln.fadeStart)) / float64(fontFadeDur)
				if t < 1 {
					return true
				}

				next, swap := advanceTextPhase(ln.phase, t)
				if swap {
					// Fade-out complete: either swap in pending and fade-in, or clear if pending is empty.
					if ln.pendingTex == nil && strings.TrimSpace(ln.pendingStr) == "" {
						clearLine(ln)
						return true
					}
					if ln.currTex != nil {
						ln.currTex.Destroy()
						ln.currTex = nil
					}
					ln.currTex, ln.currW, ln.currH = ln.pendingTex, ln.pendingW, ln.pendingH
					ln.currStr = ln.pendingStr
					ln.pendingTex = nil
					ln.pendingStr = ""
					ln.pendingW, ln.pendingH = 0, 0

					ln.phase = next
					ln.fadeStart = time.Now()
					if ln.key == "zone" {
						ln.hideAt = time.Now().Add(zoneOverlayVisibleFor)
					}
					return true
				}
				// Fade-in complete (or unexpected phase): settle.
				ln.phase = next
				return true
			}

			needRender := false
			needRender = progressLine(&zoneLine) || needRender
			needRender = progressLine(&titleLine) || needRender
			needRender = progressLine(&artistLine) || needRender
			needRender = progressLine(&albumLine) || needRender
			needRender = zoneNeedsHide || needRender

			if needRender {
				if err := render(); err != nil {
					return err
				}
			}

			for {
				e := sdl.PollEvent()
				if e == nil {
					break
				}
				switch e := e.(type) {
				case *sdl.QuitEvent:
					return nil
				case *sdl.WindowEvent:
					// Keep output-size info up to date (window resize, display changes, etc.).
					switch e.Event {
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
				case *sdl.KeyboardEvent:
					if d.EventCh == nil {
						break
					}
					// Only react on key down (and ignore repeats).
					if e.Type != sdl.KEYDOWN || e.Repeat != 0 {
						break
					}
					var kind EventKind
					switch e.Keysym.Sym {
					case sdl.K_LEFT:
						kind = EventPrevZone
					case sdl.K_RIGHT:
						kind = EventNextZone
					default:
						break
					}
					if kind != 0 {
						select {
						case d.EventCh <- Event{Kind: kind}:
						default:
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
