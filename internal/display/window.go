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
	config                 *Window
	ctx                    context.Context
	scenes                 <-chan Update
	cover                  coverLayer
	text                   *textEngine
	lines                  [4]textLine
	enabled                [4]bool
	width, height          int
	scale                  float64
	now                    time.Time
	timer                  *time.Timer
	err                    error
	presented, minimumMode bool
	status                 *Status
	statusRows             []statusRow
	statusFadeStart        time.Time
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
	// Sleep between external updates and animation/overlay deadlines.
	// Bootstrap a frame before entering event-driven mode: the application
	// needs Layout's size before it can publish its first artwork request.
	ebiten.SetFPSMode(ebiten.FPSModeVsyncOn)
	localCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	scenes := make(chan Update, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-localCtx.Done():
				ebiten.ScheduleFrame()
				return
			case scene, ok := <-updates:
				if !ok {
					cancel()
					ebiten.ScheduleFrame()
					return
				}
				Publish(scenes, scene)
				ebiten.ScheduleFrame()
			}
		}
	}()
	defer func() { cancel(); <-done }()
	g := &windowGame{config: d, ctx: localCtx, scenes: scenes, scale: 1, enabled: [4]bool{d.ShowTitle, d.ShowArtist, d.ShowAlbum, d.ShowZone}}
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
	if g.timer != nil {
		g.timer.Stop()
	}
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
	if g.presented && !g.minimumMode {
		ebiten.SetFPSMode(ebiten.FPSModeVsyncOffMinimum)
		g.minimumMode = true
	}
	if g.err != nil {
		return g.err
	}
	if g.ctx.Err() != nil || inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		return ebiten.Termination
	}
	g.now = time.Now()
	select {
	case scene := <-g.scenes:
		g.setStatus(scene.Status, g.now)
		firstCover := g.cover.fadeInNext && scene.Artwork != nil
		g.cover.set(scene.Artwork, scene.NoFade, g.now)
		values := [4]string{"", "", "", scene.Zone}
		if scene.NowPlaying != nil {
			values[0] = scene.NowPlaying.Title
			values[1] = scene.NowPlaying.Artist
			values[2] = scene.NowPlaying.Album
		}
		for i, value := range values {
			if !g.enabled[i] {
				value = ""
			}
			g.lines[i].animation.set(strings.TrimSpace(value), g.now, scene.NoFade)
			if firstCover && value != "" && g.cover.duration > 0 {
				g.lines[i].animation.phase = textPhaseFadeIn
				g.lines[i].animation.from = 0
				g.lines[i].animation.start = g.now
				g.lines[i].animation.entry = true
			}
		}
	default:
	}
	if g.status != nil {
		g.schedule()
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
	for i := range g.lines {
		g.lines[i].animation.advance(g.now)
	}
	g.schedule()
	return nil
}
func (g *windowGame) schedule() {
	delay := time.Duration(0)
	if g.status != nil && g.status.FadeOut && g.now.Sub(g.statusFadeStart) < g.cover.duration {
		delay = time.Second / 60
	}
	if g.cover.active(g.now) {
		delay = time.Second / 60
	}
	for _, row := range g.statusRows {
		if g.now.Sub(row.start) < g.cover.duration {
			delay = time.Second / 60
		}
	}
	for i := range g.lines {
		a := &g.lines[i].animation
		if a.phase != textPhaseNone {
			delay = time.Second / 60
		}
		if !a.expires.IsZero() {
			d := max(time.Millisecond, a.expires.Sub(g.now))
			if delay == 0 || d < delay {
				delay = d
			}
		}
	}
	if g.timer != nil {
		g.timer.Stop()
	}
	if delay > 0 {
		g.timer = time.AfterFunc(delay, ebiten.ScheduleFrame)
	}
}
func (g *windowGame) Layout(w, h int) (int, int) {
	scale := ebiten.Monitor().DeviceScaleFactor()
	width, height := max(1, int(math.Round(float64(w)*scale))), max(1, int(math.Round(float64(h)*scale)))
	if width != g.width || height != g.height || scale != g.scale {
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
	defer func() { g.presented = true }()
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
	lineHeight := math.Ceil(g.text.size * g.scale * 1.5)
	count := 0
	for i := 0; i < 3; i++ {
		if g.lines[i].animation.value != "" {
			count++
		}
	}
	y := float64(g.height) - pad - float64(count)*lineHeight
	for i := range g.lines {
		line := &g.lines[i]
		if line.animation.value != line.rendered {
			if line.image != nil {
				line.image.Deallocate()
				line.image = nil
			}
			if line.animation.value != "" {
				pixels, missing, err := g.text.raster(line.animation.value, max(1, g.width-int(2*pad)), g.scale)
				if err != nil {
					g.err = err
					ebiten.ScheduleFrame()
					return
				}
				if missing > 0 {
					slog.Warn("some metadata glyphs unavailable; install Noto fonts", "missing", missing)
				}
				line.image = ebiten.NewImageFromImage(pixels)
			}
			line.rendered = line.animation.value
		}
		if line.image == nil {
			continue
		}
		ly := y
		if i == 3 {
			ly = pad
		} else {
			y += lineHeight
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
