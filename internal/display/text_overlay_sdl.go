//go:build sdl

package display

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"roon-cover/internal/roon"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

func buildOverlayLines(np *roon.NowPlaying, showTitle, showArtist, showAlbum bool) []string {
	if np == nil {
		return nil
	}
	out := make([]string, 0, 3)
	if showTitle && strings.TrimSpace(np.Title) != "" {
		out = append(out, strings.TrimSpace(np.Title))
	}
	if showArtist && strings.TrimSpace(np.Artist) != "" {
		out = append(out, strings.TrimSpace(np.Artist))
	}
	if showAlbum && strings.TrimSpace(np.Album) != "" {
		out = append(out, strings.TrimSpace(np.Album))
	}
	return out
}

func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func resolveFontPath(preferred string) (string, error) {
	if strings.TrimSpace(preferred) != "" {
		p := strings.TrimSpace(preferred)
		if !filepath.IsAbs(p) {
			if wd, err := os.Getwd(); err == nil {
				p = filepath.Join(wd, p)
			}
		}
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("font not found: %q", preferred)
		}
		return p, nil
	}

	for _, p := range defaultFontCandidates() {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no default system font found; specify one via --font / ROON_COVER_DISPLAY_FONT")
}

func defaultFontCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Library/Fonts/Arial.ttf",
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/System/Library/Fonts/Supplemental/Helvetica.ttf",
			"/System/Library/Fonts/Helvetica.ttc",
		}
	case "windows":
		win := os.Getenv("WINDIR")
		if win == "" {
			win = `C:\Windows`
		}
		return []string{
			filepath.Join(win, "Fonts", "arial.ttf"),
			filepath.Join(win, "Fonts", "segoeui.ttf"),
			filepath.Join(win, "Fonts", "tahoma.ttf"),
		}
	default: // linux, etc.
		return []string{
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
			"/usr/share/fonts/truetype/freefont/FreeSans.ttf",
		}
	}
}

func renderTextBlock(ren *sdl.Renderer, font *ttf.Font, lines []string) (*sdl.Texture, int32, int32, error) {
	if ren == nil || font == nil || len(lines) == 0 {
		return nil, 0, 0, nil
	}

	// Render each line to a surface first to measure.
	color := sdl.Color{R: 255, G: 255, B: 255, A: 255}
	shadowColor := sdl.Color{R: 0, G: 0, B: 0, A: 160}
	lineSurfaces := make([]*sdl.Surface, 0, len(lines))
	shadowSurfaces := make([]*sdl.Surface, 0, len(lines))
	defer func() {
		for _, s := range lineSurfaces {
			if s != nil {
				s.Free()
			}
		}
		for _, s := range shadowSurfaces {
			if s != nil {
				s.Free()
			}
		}
	}()

	var w int32
	var h int32
	lineGap := int32(6)
	for _, line := range lines {
		s, err := font.RenderUTF8Blended(line, color)
		if err != nil {
			return nil, 0, 0, err
		}
		lineSurfaces = append(lineSurfaces, s)

		ss, err := font.RenderUTF8Blended(line, shadowColor)
		if err != nil {
			return nil, 0, 0, err
		}
		shadowSurfaces = append(shadowSurfaces, ss)

		if s.W > w {
			w = s.W
		}
		h += s.H
	}
	if len(lineSurfaces) > 1 {
		h += int32(len(lineSurfaces)-1) * lineGap
	}
	if w <= 0 || h <= 0 {
		return nil, 0, 0, nil
	}

	// Compose into a single ARGB surface.
	target, err := sdl.CreateRGBSurfaceWithFormat(0, w, h, 32, sdl.PIXELFORMAT_ARGB8888)
	if err != nil {
		return nil, 0, 0, err
	}
	defer target.Free()

	// Transparent background (we draw the backdrop rectangle separately).
	_ = target.FillRect(nil, 0)

	// Drop shadow: draw a semi-transparent black text slightly offset behind the main text.
	// To make it feel "softer" without a full blur pass, we draw it a few times with small offsets.
	shadowOffsets := []sdl.Point{
		{X: 2, Y: 2},
		{X: 2, Y: 3},
		{X: 3, Y: 2},
		{X: 3, Y: 3},
	}

	y := int32(0)
	for i, s := range lineSurfaces {
		ss := shadowSurfaces[i]
		_ = ss.SetBlendMode(sdl.BLENDMODE_BLEND)
		for _, off := range shadowOffsets {
			dstShadow := sdl.Rect{X: int32(off.X), Y: y + int32(off.Y), W: ss.W, H: ss.H}
			if err := ss.Blit(nil, target, &dstShadow); err != nil {
				return nil, 0, 0, err
			}
		}

		dst := sdl.Rect{X: 0, Y: y, W: s.W, H: s.H}
		_ = s.SetBlendMode(sdl.BLENDMODE_BLEND)
		if err := s.Blit(nil, target, &dst); err != nil {
			return nil, 0, 0, err
		}
		y += s.H + lineGap
	}

	tex, err := ren.CreateTextureFromSurface(target)
	if err != nil {
		return nil, 0, 0, err
	}
	_ = tex.SetBlendMode(sdl.BLENDMODE_BLEND)
	return tex, w, h, nil
}
