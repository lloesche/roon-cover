package app

import (
	"context"
	"roon-cover/internal/display"
	"roon-cover/internal/roon"
)

type artworkRequest struct {
	ctx      context.Context
	key      string
	imageKey roon.ImageKey
	size     int
	zone     string
}
type artworkResult struct {
	key   string
	asset *display.Artwork
	err   error
}

// One worker plus a latest-request mailbox bounds downloads and decoding work.
// The controller also checks the key, since cancellation can race a completed fetch.
func (c *Controller) loadArtwork(ctx context.Context, jobs <-chan artworkRequest, results chan<- artworkResult) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-jobs:
			if job.ctx.Err() != nil {
				continue
			}
			data, mime, err := c.Source.FetchImage(job.ctx, c.Core, job.imageKey, roon.ImageFetchOptions{Size: job.size})
			var asset *display.Artwork
			if err == nil && job.ctx.Err() == nil {
				asset, err = display.DecodeArtwork(job.key, data)
			}
			if job.ctx.Err() != nil {
				err = job.ctx.Err()
			}
			if err == nil && c.Options.SaveArtwork != nil {
				c.Options.SaveArtwork(job.zone, data, mime)
			}
			select {
			case results <- artworkResult{key: job.key, asset: asset, err: err}:
			case <-ctx.Done():
				return
			}
		}
	}
}
