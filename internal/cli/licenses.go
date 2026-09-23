package cli

import (
	"fmt"
	"roon-cover/internal/display"

	"github.com/spf13/cobra"
)

func newLicensesCmd() *cobra.Command {
	return &cobra.Command{Use: "licenses", Short: "Show the bundled Inter font's copyright and license", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprint(cmd.OutOrStdout(), display.InterLicense)
		return err
	}}
}
