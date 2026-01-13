//go:build sdl

package display

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

type fontProbe struct {
	missingW   int32
	missingH   int32
	missingHh  uint64
	hasMissing bool

	runeOK map[rune]bool
}

var (
	fontProbeMu sync.Mutex
	fontProbes  = map[*ttf.Font]*fontProbe{}
)

var runeFallback = map[rune]string{
	// Dashes/hyphens/minus.
	'\u2010': "-", // hyphen
	'\u2011': "-", // non-breaking hyphen
	'\u2012': "-", // figure dash
	'\u2013': "-", // en dash
	'\u2014': "-", // em dash
	'\u2212': "-", // minus sign
	'\u00ad': "",  // soft hyphen
	// Quotes.
	'\u2018': "'",
	'\u2019': "'",
	'\u201c': "\"",
	'\u201d': "\"",
	// Spaces.
	'\u00a0': " ", // NBSP
	// Ellipsis.
	'\u2026': "...",
}

// SDL_ttf typically returns a "missing glyph" (tofu) for runes not present in the font.
// Unfortunately the go-sdl2/ttf wrapper doesn't expose TTF_GlyphIsProvided32, so we detect tofu
// by comparing rendered pixel signatures against a known-missing rune (U+10FFFF).
func fontSupportsText(f *ttf.Font, text string) bool {
	if f == nil {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}

	// Fast path: ASCII only.
	asciiOnly := true
	for _, r := range text {
		if r > 0x7f {
			asciiOnly = false
			break
		}
	}
	if asciiOnly {
		return true
	}

	p := getFontProbe(f)
	for _, r := range text {
		if r <= 0x7f || r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if ok, seen := p.runeOK[r]; seen {
			if !ok {
				return false
			}
			continue
		}
		ok := fontHasRune(f, p, r)
		p.runeOK[r] = ok
		if !ok {
			return false
		}
	}
	return true
}

func getFontProbe(f *ttf.Font) *fontProbe {
	fontProbeMu.Lock()
	defer fontProbeMu.Unlock()
	if p, ok := fontProbes[f]; ok {
		return p
	}
	p := &fontProbe{runeOK: make(map[rune]bool, 64)}
	fontProbes[f] = p
	return p
}

func fontHasRune(f *ttf.Font, p *fontProbe, r rune) bool {
	ensureMissingSig(f, p)
	if !p.hasMissing {
		// If we couldn't compute a missing signature, assume font can render (best-effort).
		return true
	}
	w, h, hh, ok := renderRuneSig(f, r)
	if !ok {
		return false
	}
	// If it renders exactly like the missing glyph, treat as missing.
	return !(w == p.missingW && h == p.missingH && hh == p.missingHh)
}

func ensureMissingSig(f *ttf.Font, p *fontProbe) {
	if p.hasMissing {
		return
	}
	// U+10FFFF is a valid Unicode scalar value but not assigned; it should render as tofu in most fonts.
	w, h, hh, ok := renderRuneSig(f, rune(0x10ffff))
	if !ok {
		// Try another unlikely codepoint.
		w, h, hh, ok = renderRuneSig(f, rune(0x0378)) // unassigned
	}
	if ok {
		p.missingW, p.missingH, p.missingHh = w, h, hh
		p.hasMissing = true
	}
}

func renderRuneSig(f *ttf.Font, r rune) (w, h int32, hh uint64, ok bool) {
	// Render the rune by itself and hash its pixel data.
	color := sdl.Color{R: 255, G: 255, B: 255, A: 255}
	s, err := f.RenderUTF8Blended(string(r), color)
	if err != nil || s == nil {
		return 0, 0, 0, false
	}
	defer s.Free()
	return surfaceSig(s)
}

func surfaceSig(s *sdl.Surface) (w, h int32, hh uint64, ok bool) {
	if s == nil || s.W <= 0 || s.H <= 0 || s.Pitch <= 0 {
		return 0, 0, 0, false
	}
	size := int(s.H) * int(s.Pitch)
	if size <= 0 {
		return 0, 0, 0, false
	}
	b := s.Pixels()
	if len(b) < size {
		return 0, 0, 0, false
	}
	b = b[:size]
	hh64 := fnv.New64a()
	_, _ = hh64.Write(b)
	return s.W, s.H, hh64.Sum64(), true
}

func pickFontForText(fonts []*ttf.Font, text string) *ttf.Font {
	for _, f := range fonts {
		if fontSupportsText(f, text) {
			return f
		}
	}
	if len(fonts) > 0 {
		return fonts[0]
	}
	return nil
}

func anyFontHasRune(fonts []*ttf.Font, r rune) bool {
	// Treat ASCII control/space as always OK.
	if r <= 0x7f {
		return true
	}
	for _, f := range fonts {
		if f == nil {
			continue
		}
		p := getFontProbe(f)
		if fontHasRune(f, p, r) {
			return true
		}
	}
	return false
}

// sanitizeTextForFonts performs a last-resort substitution ONLY for runes that none of the fonts can render.
// This preserves true Unicode whenever possible (the user's preferred behavior).
func sanitizeTextForFonts(fonts []*ttf.Font, text string) (string, bool) {
	if text == "" || len(fonts) == 0 {
		return text, false
	}

	changed := false
	var b strings.Builder
	b.Grow(len(text))

	for _, r := range text {
		// Keep whitespace as-is.
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			b.WriteRune(r)
			continue
		}
		if anyFontHasRune(fonts, r) {
			b.WriteRune(r)
			continue
		}

		if repl, ok := runeFallback[r]; ok {
			b.WriteString(repl)
			changed = true
			continue
		}

		// Unknown unsupported rune: replace with a conservative placeholder.
		b.WriteRune('?')
		changed = true
	}

	out := b.String()
	if changed {
		out = strings.TrimSpace(out)
	}
	return out, changed
}

func renderTextLine(ren *sdl.Renderer, fonts []*ttf.Font, line string) (*sdl.Texture, int32, int32, error) {
	if ren == nil {
		return nil, 0, 0, nil
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, 0, 0, nil
	}

	font := pickFontForText(fonts, line)
	if font == nil || !fontSupportsText(font, line) {
		// If none of our fonts can render the original string, do a last-resort substitution of only the missing runes.
		if sanitized, changed := sanitizeTextForFonts(fonts, line); changed && sanitized != "" {
			line = sanitized
			font = pickFontForText(fonts, line)
		}
	}
	if font == nil {
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
			// Wider Unicode coverage on many macOS installs:
			"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
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
			// Often helps for “missing glyph” boxes:
			filepath.Join(win, "Fonts", "seguisym.ttf"),
			filepath.Join(win, "Fonts", "tahoma.ttf"),
		}
	default: // linux, etc.
		return []string{
			// Noto tends to have very broad coverage when installed.
			"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
			"/usr/share/fonts/truetype/freefont/FreeSans.ttf",
		}
	}
}
