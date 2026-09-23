package cli

import (
	"github.com/spf13/cobra"
	"roon-cover/internal/roon"
)

func newRoonPairCmd() *cobra.Command {
	return &cobra.Command{Use: "pair", Short: "Discover, pair, and save this extension's credentials", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		client := roon.NewClient(roon.Config{DisplayName: "roon-cover"}, roon.WithLogger(LoggerFromContext(cmd.Context())))
		_, err := ensureCoreAndPaired(cmd, client)
		return err
	}}
}
