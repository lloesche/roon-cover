package display

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestDecodePreservesStraightAlpha(t *testing.T) {
	p := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	p.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 128})
	var b bytes.Buffer
	_ = png.Encode(&b, p)
	a, err := DecodeArtwork("a", b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Pixels.NRGBAAt(0, 0); got.R != 255 || got.A != 128 {
		t.Fatal(got)
	}
}
func TestDecodeRejectsInvalidImage(t *testing.T) {
	if _, err := DecodeArtwork("x", []byte("invalid")); err == nil {
		t.Fatal("accepted invalid image")
	}
}
