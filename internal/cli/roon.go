package cli

import (
	"github.com/spf13/cobra"
)

func newRoonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "roon",
		Short: "Roon diagnostics (discovery and zones)",
	}

	cmd.AddCommand(newRoonDiscoverCmd())
	cmd.AddCommand(newRoonZonesCmd())
	return cmd
}
