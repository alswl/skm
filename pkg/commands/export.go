package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:     "export",
	Short:   "Emit a quote-safe skm deploy command for installed assets",
	Example: "  skm export",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.Export()
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		if result.Command == "" {
			return nil // nothing installed: no unscoped command (SC-008)
		}
		fmt.Fprintln(cmd.OutOrStdout(), result.Command)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
}
