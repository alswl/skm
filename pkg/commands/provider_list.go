package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var providerListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List providers in resolution order",
	Example: "  skm provider list --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		rep := svc.ProviderList()
		if flagJSON {
			return printJSON(cmd, rep)
		}
		for _, p := range rep.Providers {
			if p.Loaded {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-8s %s\n", p.ID, p.Kind, p.Description)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-8s FAILED: %s\n", orDashStr(p.ID), p.Kind, p.Error.Message)
			}
		}
		return nil
	},
}

func init() {
	providerCmd.AddCommand(providerListCmd)
}
