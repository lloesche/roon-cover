package display

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-text/render"
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/fontscan"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/bidi"
)

// textEngine owns Go-native shaping, system-font fallback and rasterization.
// Cached line images are uploaded once per text/size change by the GPU layer.
type textEngine struct {
	fonts     *fontscan.FontMap
	size      float64
	segmenter shaping.Segmenter
	shaper    shaping.HarfbuzzShaper
	wrapper   shaping.LineWrapper
}

func newTextEngine(path string, size int) (*textEngine, error) {
	fm := fontscan.NewFontMap(fontScanLogger{})
	if err := fm.UseSystemFonts(""); err != nil {
		slog.Warn("system font scan failed; using bundled fallback", "error", err)
	} else {
		slog.Info("Using installed fonts for song titles and artist names")
	}
	if err := fm.AddFont(bytes.NewReader(goregular.TTF), "bundled-go-regular", "roon-fallback"); err != nil {
		return nil, err
	}
	families := []string{"sans-serif", "roon-fallback"}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("custom font: %w", err)
		}
		if err := fm.AddFont(bytes.NewReader(data), path, "roon-custom"); err != nil {
			return nil, fmt.Errorf("custom font: %w", err)
		}
		families = append([]string{"roon-custom"}, families...)
	}
	fm.SetQuery(fontscan.Query{Families: families})
	return &textEngine{fonts: fm, size: float64(size)}, nil
}

// Keep font discovery diagnostics out of ordinary startup output. Handle the
// directory list before the library quotes it, avoiding nested log escaping.
type fontScanLogger struct{}

func (fontScanLogger) Printf(format string, args ...interface{}) {
	if format == "using system font dirs %q" && len(args) == 1 {
		if dirs, ok := args[0].([]string); ok {
			seen := make(map[string]bool)
			for _, dir := range dirs {
				path := filepath.ToSlash(filepath.Clean(dir))
				key := path
				if runtime.GOOS == "windows" {
					key = strings.ToLower(key)
				}
				if !seen[key] {
					slog.Debug("Looking for fonts", "directory", path)
					seen[key] = true
				}
			}
			return
		}
	}
	slog.Debug(fmt.Sprintf(format, args...))
}
func (t *textEngine) close() {}
func (t *textEngine) raster(value string, width int, scale float64) (*image.NRGBA, int, error) {
	if !utf8.ValidString(value) || len(value) > 64<<10 {
		return nil, 0, fmt.Errorf("metadata must be valid UTF-8 and at most 64 KiB")
	}
	value = strings.Join(strings.Fields(value), " ")
	width = max(1, min(width, 16384))
	scale = max(.5, min(scale, 4))
	size := int(math.Round(t.size * scale))
	if value == "" {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1)), 0, nil
	}
	runes := []rune(value)
	direction := di.DirectionLTR
	var paragraph bidi.Paragraph
	if _, err := paragraph.SetString(value); err != nil {
		return nil, 0, err
	}
	ordering, err := paragraph.Order()
	if err != nil {
		return nil, 0, err
	}
	if ordering.Direction() == bidi.RightToLeft {
		direction = di.DirectionRTL
	}
	input := shaping.Input{Text: runes, RunEnd: len(runes), Size: fixed.I(size), Direction: direction}
	inputs := t.segmenter.Split(input, t.fonts)
	runs := make([]shaping.Output, len(inputs))
	for i, in := range inputs {
		runs[i] = t.shaper.Shape(in)
	}
	ellipsis := shaping.Input{Text: []rune{'…'}, RunEnd: 1, Size: input.Size, Direction: direction, Face: t.fonts.ResolveFace('…')}
	config := shaping.WrapConfig{Direction: direction, TruncateAfterLines: 1, BreakPolicy: shaping.Always}.WithTruncator(&t.shaper, ellipsis)
	t.wrapper.Prepare(config, runes, shaping.NewSliceIterator(runs))
	wrapped, _ := t.wrapper.WrapNextLine(max(1, width-4))
	line := wrapped.Line
	sort.Slice(line, func(i, j int) bool { return line[i].VisualIndex < line[j].VisualIndex })
	ascent, descent, advance, missing := 0, 0, 0, 0
	for _, run := range line {
		ascent = max(ascent, run.LineBounds.Ascent.Ceil(), run.GlyphBounds.Ascent.Ceil())
		descent = max(descent, (-run.LineBounds.Descent).Ceil(), (-run.GlyphBounds.Descent).Ceil())
		advance += run.Advance.Ceil()
		for _, g := range run.Glyphs {
			if g.GlyphID == 0 {
				missing++
			}
		}
	}
	out := image.NewNRGBA(image.Rect(0, 0, min(width, max(1, advance+4)), max(1, ascent+descent+4)))
	renderer := render.Renderer{FontSize: float32(size), Color: color.White}
	x := 2
	for _, run := range line {
		x = drawTextRun(&renderer, run, out, x, ascent+2)
	}
	return out, missing, nil
}

// go-text/render handles outlines, SVG and bitmap glyphs. Compose COLRv0
// palette layers here as shaped outlines, preserving the library's positioning.
func drawTextRun(r *render.Renderer, run shaping.Output, dst *image.NRGBA, x, y int) int {
	start := 0
	for i, glyph := range run.Glyphs {
		data, ok := run.Face.GlyphDataColor(glyph.GlyphID)
		if !ok {
			continue
		}
		prefix := run
		prefix.Glyphs = run.Glyphs[start:i]
		if len(prefix.Glyphs) > 0 {
			x = r.DrawShapedRunAt(prefix, dst, x, y)
		}
		if layers, ok := data.Paint.(tables.PaintColrLayersResolved); ok {
			for _, layer := range layers {
				r.Color = color.White
				if layer.PaletteIndex != 0xffff && len(run.Face.CPAL) > 0 && int(layer.PaletteIndex) < len(run.Face.CPAL[0]) {
					c := run.Face.CPAL[0][layer.PaletteIndex]
					r.Color = color.NRGBA{R: c.Red, G: c.Green, B: c.Blue, A: c.Alpha}
				}
				g := glyph
				g.GlyphID = font.GID(layer.GlyphID)
				part := run
				part.Glyphs = []shaping.Glyph{g}
				r.DrawShapedRunAt(part, dst, x, y)
			}
		} else {
			// COLRv1 paint graphs are not supported by this rasterizer. Use
			// the font's monochrome outline instead of silently dropping it.
			// Copies keep the shared font and shaping caches unmodified.
			outlineFont := *run.Face.Font
			outlineFont.COLR = nil
			outlineFace := *run.Face
			outlineFace.Font = &outlineFont
			part := run
			part.Face = &outlineFace
			part.Glyphs = []shaping.Glyph{glyph}
			r.DrawShapedRunAt(part, dst, x, y)
		}
		r.Color = color.White
		x += glyph.Advance.Ceil()
		start = i + 1
	}
	run.Glyphs = run.Glyphs[start:]
	if len(run.Glyphs) > 0 {
		x = r.DrawShapedRunAt(run, dst, x, y)
	}
	return x
}
