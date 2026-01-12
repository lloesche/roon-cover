package display

import "context"

// Display runs a UI loop and consumes Update messages.
// Implementations should return when ctx is cancelled or the window is closed.
type Display interface {
	Run(ctx context.Context, updates <-chan Update) error
}
