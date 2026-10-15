package commands

import (
	"github.com/alswl/skm/skm/pkg/engines"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:     "update NAME",
	Short:   "Refresh an entry from its recorded origin",
	Example: "  skm update review --json",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		accept, _ := cmd.Flags().GetBool("accept")
		abort, _ := cmd.Flags().GetBool("abort")
		rollback, _ := cmd.Flags().GetBool("rollback")
		var result *engines.UpdateResult
		switch {
		case accept:
			result, err = svc.AcceptUpdate(cmd.Context(), args[0], flagDryRun)
		case abort:
			result, err = svc.CancelUpdate(cmd.Context(), args[0], flagDryRun)
		case rollback:
			result, err = svc.RollbackUpdate(cmd.Context(), args[0], flagDryRun)
		default:
			result, err = svc.Update(cmd.Context(), args[0], services.UpdateOptions{DryRun: flagDryRun})
		}
		return printSyncResult(cmd, result, err)
	},
}

func init() {
	updateCmd.Flags().Bool("accept", false, "accept the manually prepared candidate")
	updateCmd.Flags().Bool("abort", false, "discard the failed attempt, preserving the current version")
	updateCmd.Flags().Bool("rollback", false, "restore the version and baseline before the last successful update")
	updateCmd.MarkFlagsMutuallyExclusive("accept", "abort", "rollback")
	rootCmd.AddCommand(updateCmd)
}
