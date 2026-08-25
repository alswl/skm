package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var targetPluginListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List discovered Target plugins",
	Example: "  skm target plugin list --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		rep := svc.TargetPluginList()
		if flagJSON {
			return printJSON(cmd, rep)
		}
		for _, p := range rep.Plugins {
			if p.Loaded {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-8v %s\n", p.ID, p.Kinds, p.Description)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s FAILED: %s\n", orDashStr(p.ID), p.Error.Message)
			}
		}
		return nil
	},
}

func init() {
	targetPluginCmd.AddCommand(targetPluginListCmd)
}
