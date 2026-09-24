package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Back up and restore local repository state (not for sharing)",
}

var backupCreateCmd = &cobra.Command{
	Use:   "create [file]",
	Short: "Create a local snapshot of the current repository state",
	Long:  "Create a local snapshot of the current repository state.\n\nWrites to the given file, or to a timestamped file under the config directory when none is given.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.CreateBackup(cmd.Context(), firstArg(args))
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "created backup %s\n", result.Path)
		return nil
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore [file]",
	Short: "Restore entries from a local backup file (defaults to the most recent)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.RestoreBackup(cmd.Context(), firstArg(args))
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

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func init() {
	backupCmd.AddCommand(backupCreateCmd, backupRestoreCmd)
	rootCmd.AddCommand(backupCmd)
}
