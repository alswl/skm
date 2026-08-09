package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

// externalReport is the stable report for actions on a discovered external
// skill.  It deliberately carries the path supplied by the caller so scripts
// can associate a result with a preceding `skm discover --json` row.
type externalReport struct {
	Action  string `json:"action"`
	Name    string `json:"name,omitempty"`
	Target  string `json:"target,omitempty"`
	Path    string `json:"path"`
	DryRun  bool   `json:"dry_run"`
	Success bool   `json:"success"`
}

var adoptCmd = &cobra.Command{
	Use:     "adopt PATH...",
	Short:   "Import external skills and replace them with managed installs",
	Example: "  skm discover --json\n  skm adopt ~/.codex/skills/review --json",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		reports := make([]externalReport, 0, len(args))
		for _, path := range args {
			if flagDryRun {
				reports = append(reports, externalReport{Action: "adopt", Path: path, DryRun: true, Success: true})
				continue
			}
			result, err := svc.AdoptExternal(cmd.Context(), path, flagForce)
			if err != nil {
				return err
			}
			reports = append(reports, externalReport{Action: "adopt", Name: result.Name, Target: result.Target, Path: result.Path, Success: true})
		}
		return printExternalReports(cmd, reports)
	},
}

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

func printExternalReports(cmd *cobra.Command, reports []externalReport) error {
	if flagJSON {
		return printJSON(cmd, reports)
	}
	for _, report := range reports {
		verb := report.Action + "ed"
		if report.Action == "delete-external" {
			verb = "deleted external"
		}
		if report.DryRun {
			verb = "would " + report.Action
		}
		if report.Name != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %q -> %s (%s)\n", verb, report.Name, report.Path, report.Target)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", verb, report.Path)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(adoptCmd, deleteExternalCmd)
}
