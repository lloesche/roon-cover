package display

import "image"

// Update is a complete desired scene. Assets are immutable; nil means blank.
type Update struct {
	Zone       string
	NowPlaying *Metadata
	Artwork    *Artwork
	NoFade     bool
	Status     *Status
}

// Status is a startup screen; it does not depend on metadata overlay flags.
type Status struct {
	Title, Detail, Hint string
}

type Metadata struct{ Title, Artist, Album string }
type Artwork struct {
	Key    string
	Pixels *image.NRGBA
}

// Publish replaces an obsolete complete scene. Only the owning producer calls it.
func Publish(ch chan Update, scene Update) {
	select {
	case ch <- scene:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- scene:
	default:
	}
}

// ScreenInfo describes the physical render output size and chosen display.
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

// Event carries renderer input back to the application controller.
type Event struct {
	Kind EventKind
}
