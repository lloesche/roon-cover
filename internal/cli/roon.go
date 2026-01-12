package cli

import (
	"github.com/spf13/cobra"
)

func newRoonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "roon",
		Short: "Roon extension operations (discovery, pairing, zones)",
	}

	cmd.AddCommand(newRoonDiscoverCmd())
	cmd.AddCommand(newRoonPairCmd())
	cmd.AddCommand(newRoonZonesCmd())
	return cmd
}
