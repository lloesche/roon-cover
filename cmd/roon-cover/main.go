package main

import (
	"os"

	"roon-cover/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
