package display

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
)

const MaxArtworkBytes = 24 << 20
const MaxArtworkPixels = 16 << 20

// DecodeArtwork runs in the artwork worker to keep image decoding off the graphics
// thread. NRGBA preserves straight alpha for texture upload.
func DecodeArtwork(key string, data []byte) (*Artwork, error) {
	if len(data) > MaxArtworkBytes {
		return nil, fmt.Errorf("artwork exceeds %d bytes", MaxArtworkBytes)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxArtworkPixels {
		return nil, fmt.Errorf("artwork dimensions exceed limit")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	draw.Draw(pixels, pixels.Bounds(), img, img.Bounds().Min, draw.Src)
	return &Artwork{Key: key, Pixels: pixels}, nil
}
