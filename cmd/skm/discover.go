package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var discoverFlags struct {
	source string
}

var discoverCmd = &cobra.Command{
	Use:     "discover",
	Short:   "List external unmanaged skills in install targets",
	Example: "  skm discover --source ~/.codex/skills --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result := svc.Discover(discoverFlags.source)
		if flagJSON {
			return printJSON(cmd, result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "source: %s\n", result.Source)
		for _, f := range result.Found {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s -> %s\n", f.Name, f.Path)
		}
		if len(result.Found) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "  (none)")
		}
		return nil
	},
}

func init() {
	discoverCmd.Flags().StringVar(&discoverFlags.source, "source", "", "scan only this directory instead of all skill targets")
	rootCmd.AddCommand(discoverCmd)
}
