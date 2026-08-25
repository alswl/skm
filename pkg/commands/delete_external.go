package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

var deleteExternalCmd = &cobra.Command{
	Use:     "delete-external PATH...",
	Short:   "Permanently remove external unmanaged skills (requires --force)",
	Example: "  skm delete-external ~/.codex/skills/review --force --json",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !flagForce {
			return common.WithExitCode(fmt.Errorf("delete-external: --force is required"), common.ExitError)
		}
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		reports := make([]externalReport, 0, len(args))
		for _, path := range args {
			if !flagDryRun {
				if err := svc.DeleteExternal(path); err != nil {
					return err
				}
			}
			reports = append(reports, externalReport{Action: "delete-external", Path: path, DryRun: flagDryRun, Success: true})
		}
		return printExternalReports(cmd, reports)
	},
}

func init() {
	rootCmd.AddCommand(deleteExternalCmd)
}
