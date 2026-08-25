package commands

import (
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

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
	uninstallCmd.Flags().StringSliceVar(&installTargets, "target", nil, "target name(s); default all kind-matching")
	rootCmd.AddCommand(uninstallCmd)
}
