package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var batchUpdateCmd = &cobra.Command{
	Use:     "batch-update",
	Short:   "Refresh all active entries that have an origin",
	Example: "  skm batch-update --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result := svc.BatchUpdate(cmd.Context(), flagDryRun)
		if flagJSON {
			return printJSON(cmd, result)
		}
		fmt.Fprintf(cmd.OutOrStdout(),
			"updated=%d current=%d failed=%d skipped=%d total=%d\n",
			len(result.Updated), len(result.Current), len(result.Failed), len(result.Skipped), result.Total)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(batchUpdateCmd)
}
