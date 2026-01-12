package cli

import (
	"fmt"

	"roon-cover/internal/roon"

	"github.com/spf13/cobra"
)

func newRoonDiscoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "Discover Roon Cores on the local network",
		RunE: func(cmd *cobra.Command, args []string) error {
			l := LoggerFromContext(cmd.Context())
			client := roon.NewClient(roon.Config{
				DisplayName: "roon-cover",
			}, roon.WithLogger(l))

			cores, err := client.Discover(cmd.Context())
			if err != nil {
				return err
			}

			for _, c := range cores {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s (%s:%d) id=%s\n", c.Name, c.Host, c.Port, c.ID)
			}
			return nil
		},
	}
}
