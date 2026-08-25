package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var targetListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List configured targets",
	Example: "  skm target list --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		rep := svc.TargetList()
		if flagJSON {
			return printJSON(cmd, rep)
		}
		for _, t := range rep.Targets {
			fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-10s %-8s %s\n", t.Name, t.Platform, t.PathState, t.Path)
		}
		for _, inv := range rep.Invalid {
			fmt.Fprintf(cmd.OutOrStdout(), "%-16s INVALID: %s\n", "—", inv.Reason)
		}
		return nil
	},
}

func init() {
	targetCmd.AddCommand(targetListCmd)
}
