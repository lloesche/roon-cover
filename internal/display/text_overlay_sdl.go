//go:build sdl

package display

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

func renderTextLine(ren *sdl.Renderer, font *ttf.Font, line string) (*sdl.Texture, int32, int32, error) {
	if ren == nil || font == nil {
		return nil, 0, 0, nil
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, 0, 0, nil
	}

	color := sdl.Color{R: 255, G: 255, B: 255, A: 255}

	main, err := font.RenderUTF8Blended(line, color)
	if err != nil {
		return nil, 0, 0, err
	}
	defer main.Free()

	w := main.W
	h := main.H
	if w <= 0 || h <= 0 {
		return nil, 0, 0, nil
	}

	tex, err := ren.CreateTextureFromSurface(main)
	if err != nil {
		return nil, 0, 0, err
	}
	_ = tex.SetBlendMode(sdl.BLENDMODE_BLEND)
	return tex, w, h, nil
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
