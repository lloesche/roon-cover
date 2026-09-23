package main

import (
	"os"
	"runtime"

	"roon-cover/internal/cli"
)

func main() {
	// Graphics initialization must happen on the main OS thread.
	runtime.LockOSThread()

	os.Exit(cli.Execute())
}
