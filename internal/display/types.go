package display

import "roon-cover/internal/roon"

// Update represents a new frame of data the UI should show.
// We keep media metadata with the image so we can overlay text later.
type Update struct {
	Zone  string
	State roon.ZoneState

	NowPlaying *roon.NowPlaying

	// CoverImage contains the raw bytes of the decoded cover as provided by Roon's image service.
	// If nil, the display should keep showing the last image (for now).
	CoverImage    []byte
	CoverMimeType string
}
