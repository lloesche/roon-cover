package cli

import (
	"errors"

	"roon-cover/internal/roon"

	"github.com/spf13/cobra"
)

func newRoonPairCmd() *cobra.Command {
	var coreName string

	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Pair this extension with a Roon Core",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if coreName == "" {
				return errors.New("--core-name is required for now")
			}

			l := LoggerFromContext(cmd.Context())
			client := roon.NewClient(roon.Config{
				DisplayName: "roon-cover",
			}, roon.WithLogger(l))

			// TODO: discovery + select by name + then pair.
			_, err := client.Pair(cmd.Context(), roon.Core{Name: coreName})
			return err
		},
	}

	cmd.Flags().StringVar(&coreName, "core-name", "", "Roon Core name to pair with")
	return cmd
}
