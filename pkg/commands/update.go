package commands

import (
	"fmt"

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
		result, err := svc.Update(cmd.Context(), args[0], services.UpdateOptions{DryRun: flagDryRun})
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		verb := "updated"
		if !result.Changed {
			verb = "current"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", verb, result.After)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
