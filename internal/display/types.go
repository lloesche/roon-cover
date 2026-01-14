package display

import "roon-cover/internal/roon"

// Update represents a new frame of data the UI should show.
// We keep media metadata with the image so we can overlay text later.
type Update struct {
	Zone  string
	State roon.ZoneState

	NowPlaying *roon.NowPlaying

	// ClearCover requests that the renderer clears any current/previous cover and shows a black screen.
	// This is useful when the zone is not playing (idle/paused/stopped) and we want to avoid showing stale art.
	ClearCover bool

	// CoverImage contains the raw image bytes returned by Roon's image service (e.g. JPEG/PNG).
	// If nil, the display should keep showing the last image (for now).
	CoverImage    []byte
	CoverMimeType string

	// NoFade requests that the renderer swaps to this cover immediately (even if fade is enabled).
	// Useful for "same cover, different size" refetches (e.g. after the window reports its real size).
	NoFade bool
}

// ScreenInfo describes the actual SDL render output size and chosen display.
// This allows the producer (roon fetcher) to request a suitably-sized cover image.
type ScreenInfo struct {
	DisplayIndex int
	RenderWidth  int
	RenderHeight int
}

type EventKind int

const (
	EventPrevZone EventKind = iota + 1
	EventNextZone
)

// Event is sent from the renderer (SDL) back to the producer (CLI) for user interactions.
type Event struct {
	Kind EventKind
}
