//go:build !sdl

package display

import (
	"context"
	"errors"
)

type SDLDisplay struct {
	Width        int
	Height       int
	Title        string
	Fullscreen   bool
	DisplayIndex int
}

func (d *SDLDisplay) Run(ctx context.Context, updates <-chan Update) error {
	_, _ = ctx, updates
	return errors.New("SDL display not built in; run with `-tags sdl` and ensure SDL2 is installed")
}
