package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

var backupRestoreID string

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Back up and restore local repository state (not for sharing)",
}

var backupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a local snapshot of the current repository state",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.CreateBackup(cmd.Context())
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "created backup %q (%s)\n", result.ID, result.Path)
		return nil
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore entries from a local backup (defaults to the most recent)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.RestoreBackup(cmd.Context(), backupRestoreID)
		if err != nil {
			return err
		}
		if flagJSON {
			if err := printJSON(cmd, result); err != nil {
				return err
			}
		} else {
			for _, item := range result.Items {
				if item.Reason != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %q: %s\n", item.Status, item.Path, item.Reason)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %q\n", item.Status, item.Path)
				}
				for _, report := range item.Results {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s -> %s\n", report.Target, report.Status)
				}
			}
		}
		if !result.Success {
			return common.WithExitCode(fmt.Errorf("backup restore: one or more entries were skipped or failed"), common.ExitObject)
		}
		return nil
	},
}

func init() {
	backupRestoreCmd.Flags().StringVar(&backupRestoreID, "id", "", "backup id to restore (default: most recent)")
	backupCmd.AddCommand(backupCreateCmd, backupRestoreCmd)
	rootCmd.AddCommand(backupCmd)
}
