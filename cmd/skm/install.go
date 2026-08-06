package main

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

// installTargets is shared by install/uninstall.
var installTargets []string

var installCmd = &cobra.Command{
	Use:     "install NAME",
	Short:   "Install a skill or command into kind-matching targets",
	Example: "  skm install review --target team-codex --json",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.Install(cmd.Context(), args[0], services.InstallOptions{
			Targets: installTargets,
			Force:   flagForce,
			DryRun:  flagDryRun,
		})
		if err != nil {
			return err
		}
		return printInstallReport(cmd, result)
	},
}

var uninstallCmd = &cobra.Command{
	Use:     "uninstall NAME",
	Short:   "Remove managed installs of a skill or command (never user files)",
	Example: "  skm uninstall review --target team-codex --json",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.Uninstall(cmd.Context(), args[0], services.InstallOptions{
			Targets: installTargets,
			DryRun:  flagDryRun,
		})
		if err != nil {
			return err
		}
		return printInstallReport(cmd, result)
	},
}

func init() {
	rootCmd.AddCommand(installCmd, uninstallCmd)
	for _, c := range []*cobra.Command{installCmd, uninstallCmd} {
		c.Flags().StringSliceVar(&installTargets, "target", nil, "target name(s); default all kind-matching")
	}
}

// printInstallReport emits the install/uninstall JSON contract or a compact
// human summary.
func printInstallReport(cmd *cobra.Command, r *services.InstallResult) error {
	if flagJSON {
		return printJSON(cmd, r)
	}
	for _, res := range r.Results {
		verb := r.Action
		if r.DryRun {
			verb = "would " + verb
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %q -> %s (%s)\n", verb, r.Name, res.Target, res.Status)
	}
	return nil
}
