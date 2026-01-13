package display

import "roon-cover/internal/roon"

// Update represents a new frame of data the UI should show.
// We keep media metadata with the image so we can overlay text later.
type Update struct {
	Zone  string
	State roon.ZoneState

	NowPlaying *roon.NowPlaying

	// CoverImage contains the raw image bytes returned by Roon's image service (e.g. JPEG/PNG).
	// If nil, the display should keep showing the last image (for now).
	CoverImage    []byte
	CoverMimeType string
}

// ScreenInfo describes the actual SDL render output size and chosen display.
// This allows the producer (roon fetcher) to request a suitably-sized cover image.
type ScreenInfo struct {
	DisplayIndex int
	RenderWidth  int
	RenderHeight int
}
