package cli

import (
	"fmt"
	"github.com/spf13/cobra"
	"runtime/debug"
)

// Set by release builds; local builds retain the development label.
var buildVersion = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print build identity and renderer", RunE: func(cmd *cobra.Command, args []string) error {
		revision, dirty, toolchain, engine := "unknown", "unknown", "unknown", "unknown"
		if info, ok := debug.ReadBuildInfo(); ok {
			toolchain = info.GoVersion
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					revision = s.Value
				case "vcs.modified":
					dirty = s.Value
				}
			}
			for _, dep := range info.Deps {
				if dep.Path == "github.com/hajimehoshi/ebiten/v2" {
					engine = dep.Version
				}
			}
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "roon-cover version=%s commit=%s dirty=%s go=%s renderer=Ebitengine/%s text=go-text\n", buildVersion, revision, dirty, toolchain, engine)
		return err
	}}
}
