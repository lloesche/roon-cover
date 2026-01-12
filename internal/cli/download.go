package cli

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"roon-cover/internal/roon"
)

var nonFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func maybeDownloadCoverToTemp(ctx context.Context, log *slog.Logger, client *roon.Client, core roon.Core, zone roon.Zone) {
	if log == nil {
		log = slog.Default()
	}
	if zone.NowPlaying == nil || zone.NowPlaying.ImageKey == "" {
		return
	}

	// Keep size reasonable; we can make this configurable later.
	img, mime, err := client.FetchImage(ctx, core, zone.NowPlaying.ImageKey, roon.ImageFetchOptions{Size: 800})
	if err != nil {
		log.Warn("cover download failed", "zone", zone.Name, "image_key", zone.NowPlaying.ImageKey, "err", err)
		return
	}

	ext := extForMime(mime)
	prefix := "roon-cover-" + sanitizeFilename(zone.Name) + "-*"
	pattern := prefix + ext

	f, err := os.CreateTemp(os.TempDir(), pattern)
	if err != nil {
		log.Warn("cover temp file create failed", "err", err)
		return
	}
	defer f.Close()

	if _, err := f.Write(img); err != nil {
		_ = os.Remove(f.Name())
		log.Warn("cover temp file write failed", "path", f.Name(), "err", err)
		return
	}

	abs, _ := filepath.Abs(f.Name())
	log.Info("cover downloaded to temp", "path", abs, "mime", mime, "bytes", len(img), "zone", zone.Name)
}

func extForMime(mime string) string {
	m := strings.ToLower(strings.TrimSpace(mime))
	switch m {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	default:
		// Unknown, but still useful.
		return ".bin"
	}
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "zone"
	}
	s = nonFilenameChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-._")
	if s == "" {
		return "zone"
	}
	// Avoid absurdly long filenames.
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}
