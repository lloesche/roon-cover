package display

import (
	"context"
	"fmt"
	"image/color"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Window owns all graphics resources and consumes complete immutable scenes.
type Window struct {
	Width, Height                              int
	Title                                      string
	Fullscreen                                 bool
	DisplayIndex                               int
	InfoCh                                     chan<- ScreenInfo
	EventCh                                    chan<- Event
	FadeMS                                     int
	Ease                                       string
	ShowTitle, ShowArtist, ShowAlbum, ShowZone bool
	FontPath                                   string
	FontSize                                   int
}
type textLine struct {
	animation textTransition
	image     *ebiten.Image
	rendered  string
}
type windowGame struct {
	config           *Window
	ctx              context.Context
	scenes           <-chan Update
	cover            coverLayer
	text             *textEngine
	lines            [4]textLine
	enabled          [4]bool
	width, height    int
	scale            float64
	now              time.Time
	err              error
	presented, dirty bool
	status           *Status
	statusRows       []statusRow
	statusFadeStart  time.Time
}

func (d *Window) Run(ctx context.Context, updates <-chan Update) error {
	if d.FadeMS < 0 {
		return fmt.Errorf("fade durations must be nonnegative")
	}
	ease, err := FadeEasingByName(d.Ease)
	if err != nil {
		return err
	}
	monitors := ebiten.AppendMonitors(nil)
	if d.DisplayIndex < 0 || d.DisplayIndex >= len(monitors) {
		return fmt.Errorf("display %d unavailable: found %d display(s); console framebuffer mode exposes only /dev/fb0", d.DisplayIndex, len(monitors))
	}
	ebiten.SetMonitor(monitors[d.DisplayIndex])
	ebiten.SetWindowTitle(d.Title)
	ebiten.SetWindowSize(max(1, d.Width), max(1, d.Height))
	if d.Width <= 0 || d.Height <= 0 {
		ebiten.SetWindowSize(800, 800)
	}
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetFullscreen(d.Fullscreen)
	if d.Fullscreen {
		ebiten.SetCursorMode(ebiten.CursorModeHidden)
	}
	ebiten.SetRunnableOnUnfocused(true)
	// Retain the last frame and leave it untouched while the scene is static.
	ebiten.SetVsyncEnabled(true)
	ebiten.SetScreenClearedEveryFrame(false)
	g := &windowGame{config: d, ctx: ctx, scenes: updates, scale: 1, dirty: true, enabled: [4]bool{d.ShowTitle, d.ShowArtist, d.ShowAlbum, d.ShowZone}}
	g.cover = coverLayer{duration: time.Duration(d.FadeMS) * time.Millisecond, ease: ease, width: 800, height: 800}
	defer g.close()
	// Startup instructions must be visible even when metadata overlays are off.
	g.text, err = newTextEngine(d.FontPath, max(6, d.FontSize))
	if err != nil {
		return err
	}
	for i := range g.lines {
		g.lines[i].animation = textTransition{duration: g.cover.duration, ease: ease}
	}
	g.lines[3].animation.hold = 5 * time.Second
	return ebiten.RunGame(g)
}
func (g *windowGame) close() {
	g.setStatus(nil, time.Time{})
	g.cover.close()
	for i := range g.lines {
		if g.lines[i].image != nil {
			g.lines[i].image.Deallocate()
		}
	}
	if g.text != nil {
		g.text.close()
	}
}
func (g *windowGame) Update() error {
	if g.err != nil {
		return g.err
	}
	if g.ctx.Err() != nil || inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		return ebiten.Termination
	}
	g.advanceAnimations(time.Now())
	select {
	case scene, ok := <-g.scenes:
		if !ok {
			return ebiten.Termination
		}
		g.applyScene(scene)
	default:
	}
	if g.status != nil {
		return nil
	}
	for _, key := range []struct {
		key   ebiten.Key
		event EventKind
	}{{ebiten.KeyLeft, EventPrevZone}, {ebiten.KeyRight, EventNextZone}} {
		if inpututil.IsKeyJustPressed(key.key) {
			select {
			case g.config.EventCh <- Event{Kind: key.event}:
			default:
			}
		}
	}
	return nil
}

func (g *windowGame) applyScene(scene Update) {
	g.setStatus(scene.Status, g.now)
	firstCover := g.cover.fadeInNext && scene.Artwork != nil
	g.dirty = g.dirty || scene.Artwork != g.cover.asset
	g.cover.set(scene.Artwork, scene.NoFade, g.now)
	values := [4]string{"", "", "", scene.Zone}
	if scene.NowPlaying != nil {
		values[0] = scene.NowPlaying.Title
		if artist := strings.TrimSpace(scene.NowPlaying.Artist); artist != "" {
			values[1] = "Performed by " + artist
		}
		if album := strings.TrimSpace(scene.NowPlaying.Album); album != "" {
			values[2] = "On " + album
		}
	}
	for i, value := range values {
		if !g.enabled[i] {
			value = ""
		}
		if g.lines[i].animation.set(strings.TrimSpace(value), g.now, scene.NoFade) {
			g.dirty = true
		}
		if firstCover && value != "" && g.cover.duration > 0 {
			g.lines[i].animation.fadeIn(g.now)
		}
	}
}

func (g *windowGame) advanceAnimations(now time.Time) {
	// Sample both clocks so crossing an animation deadline still paints its
	// final frame. Multiple Update calls before Draw preserve the dirty flag.
	g.dirty = g.dirty || g.animating(g.now) || g.animating(now)
	g.now = now
	if g.status == nil {
		for i := range g.lines {
			if g.lines[i].animation.advance(now) {
				g.dirty = true
			}
		}
	}
}
func (g *windowGame) animating(now time.Time) bool {
	if g.status != nil {
		if g.status.FadeOut {
			return now.Sub(g.statusFadeStart) < g.cover.duration
		}
		for _, row := range g.statusRows {
			if now.Sub(row.start) < g.cover.duration {
				return true
			}
		}
		return false
	}
	return g.cover.active(now)
}
func (g *windowGame) Layout(w, h int) (int, int) {
	scale := ebiten.Monitor().DeviceScaleFactor()
	width, height := max(1, int(math.Round(float64(w)*scale))), max(1, int(math.Round(float64(h)*scale)))
	if width != g.width || height != g.height || scale != g.scale {
		g.dirty = true
		g.clearStatusImage()
		g.width = width
		g.height = height
		g.scale = scale
		g.cover.resize(width, height, time.Now())
		for i := range g.lines {
			if g.lines[i].image != nil {
				g.lines[i].image.Deallocate()
				g.lines[i].image = nil
			}
			g.lines[i].rendered = ""
		}
		select {
		case g.config.InfoCh <- ScreenInfo{DisplayIndex: g.config.DisplayIndex, RenderWidth: width, RenderHeight: height}:
		default:
		}
	}
	return width, height
}
func (g *windowGame) Draw(screen *ebiten.Image) {
	if g.presented && !g.dirty {
		return
	}
	defer func() { g.presented = true; g.dirty = false }()
	screen.Fill(color.Black)
	if g.status != nil {
		g.drawStatus(screen)
		return
	}
	g.cover.draw(screen, g.now)
	if g.text == nil {
		return
	}
	pad := math.Round(24 * g.scale)
	// Rasterize first so positioning uses the actual shaped text heights,
	// including fallback fonts with taller glyphs.
	for i := range g.lines {
		line := &g.lines[i]
		if line.animation.value != line.rendered {
			if line.image != nil {
				line.image.Deallocate()
				line.image = nil
			}
			if line.animation.value != "" {
				scale := g.scale
				if i == 0 {
					scale *= 1.5
				}
				pixels, missing, err := g.text.raster(line.animation.value, max(1, g.width-int(2*pad)), scale)
				if err != nil {
					g.err = err
					return
				}
				if missing > 0 {
					slog.Warn("some metadata glyphs unavailable; install Noto fonts", "missing", missing)
				}
				line.image = ebiten.NewImageFromImage(pixels)
			}
			line.rendered = line.animation.value
		}
	}
	// Title, album, artist; the temporary zone label stays at the top left.
	order := [...]int{0, 2, 1}
	gap := math.Round(g.text.size * g.scale * .1)
	titleGap := math.Round(g.text.size * g.scale * .5)
	height := 0.0
	visible := 0
	for _, i := range order {
		if line := &g.lines[i]; line.image != nil {
			if visible > 0 {
				height += gap
				if g.lines[0].image != nil && visible == 1 {
					height += titleGap
				}
			}
			height += float64(line.image.Bounds().Dy())
			visible++
		}
	}
	y := float64(g.height) - pad - height
	for _, i := range [...]int{0, 2, 1, 3} {
		line := &g.lines[i]
		if line.image == nil {
			continue
		}
		ly := y
		if i == 3 {
			ly = pad
		} else {
			y += float64(line.image.Bounds().Dy()) + gap
			if i == 0 {
				y += titleGap
			}
		}
		alpha := float32(line.animation.opacity(g.now))
		shadow := &ebiten.DrawImageOptions{}
		shadow.GeoM.Translate(pad+2*g.scale, ly+2*g.scale)
		shadow.ColorScale.Scale(0, 0, 0, alpha*.7)
		screen.DrawImage(line.image, shadow)
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(pad, ly)
		opts.ColorScale.ScaleAlpha(alpha)
		screen.DrawImage(line.image, opts)
	}
}
