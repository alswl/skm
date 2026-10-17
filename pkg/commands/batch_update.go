package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"

	"github.com/spf13/cobra"
)

var batchUpdateCmd = &cobra.Command{
	Use:     "batch-update",
	Short:   "Refresh eligible Git Skills and other updatable entries",
	Example: "  skm batch-update --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result := svc.BatchUpdate(cmd.Context(), flagDryRun)
		if flagJSON {
			if err := printJSON(cmd, result); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(cmd.OutOrStdout(),
				"updated=%d current=%d failed=%d skipped=%d total=%d\n",
				len(result.Updated), len(result.Current), len(result.Failed), len(result.Skipped), result.Total)
		}
		if len(result.Failed) > 0 {
			return common.WithExitCode(fmt.Errorf("batch-update: %d failed", len(result.Failed)), result.ExitCode)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(batchUpdateCmd)
}
