package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		RunE: func(cmd *cobra.Command, args []string) error {
			info, ok := debug.ReadBuildInfo()
			if !ok || info == nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "roon-cover (build info unavailable)")
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", info.Path, info.Main.Version)
			return nil
		},
	}
}
