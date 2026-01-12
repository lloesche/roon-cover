package main

import (
	"os"
	"runtime"

	"roon-cover/internal/cli"
)

func main() {
	// Required for SDL/Cocoa on macOS: initialization must happen on the main thread.
	// Lock early to ensure Cobra commands that use SDL run on the correct thread.
	runtime.LockOSThread()

	os.Exit(cli.Execute())
}
