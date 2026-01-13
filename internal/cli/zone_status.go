package cli

import (
	"log/slog"
	"strings"

	"roon-cover/internal/roon"
)

// ZoneStatusLogger emits concise INFO logs when a zone's playback state / now playing changes.
// It is meant to be reused across modes (display, watch, default run).
type ZoneStatusLogger struct {
	lastState  roon.ZoneState
	lastKey    roon.ImageKey
	lastTitle  string
	lastArtist string
	lastAlbum  string
}

func (z *ZoneStatusLogger) Observe(log *slog.Logger, zone roon.Zone) {
	if log == nil {
		log = slog.Default()
	}

	state := zone.State

	var (
		title  string
		artist string
		album  string
		key    roon.ImageKey
	)
	if zone.NowPlaying != nil {
		title = strings.TrimSpace(zone.NowPlaying.Title)
		artist = strings.TrimSpace(zone.NowPlaying.Artist)
		album = strings.TrimSpace(zone.NowPlaying.Album)
		key = zone.NowPlaying.ImageKey
	}

	stateChanged := state != z.lastState
	trackChanged := title != z.lastTitle || artist != z.lastArtist || album != z.lastAlbum
	keyChanged := key != z.lastKey

	if !(stateChanged || trackChanged || keyChanged) {
		return
	}

	// Only log meaningful info; avoid noisy empty strings.
	attrs := []any{
		"zone", zone.Name,
		"state", state,
	}
	if title != "" || artist != "" || album != "" {
		attrs = append(attrs,
			"title", title,
			"artist", artist,
			"album", album,
		)
	}
	if key != "" {
		attrs = append(attrs, "image_key", key)
	}

	log.Info("zone status", attrs...)

	z.lastState = state
	z.lastTitle = title
	z.lastArtist = artist
	z.lastAlbum = album
	z.lastKey = key
}
