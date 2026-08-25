package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/spf13/cobra"
)

var normalizeFlags struct {
	provider string
}

// normalizeCmd exposes the same relocation operation as the TUI's “move to
// standard location” action.  Keeping it separate from the generic lifecycle
// constructor makes the destination provider explicit and scriptable.
var normalizeCmd = &cobra.Command{
	Use:     "normalize NAME",
	Short:   "Move a non-standard entry into a provider location",
	Example: "  skm normalize review --provider local --dry-run\n  skm normalize review --provider github --json",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.Normalize(cmd.Context(), args[0], normalizeFlags.provider,
			engines.LifecycleOptions{DryRun: flagDryRun})
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		verb := "normalized"
		if result.DryRun {
			verb = "would normalize"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %q -> %s\n", verb, result.Name, result.Path)
		return nil
	},
}

func init() {
	normalizeCmd.Flags().StringVar(&normalizeFlags.provider, "provider", "local", "destination provider id")
	rootCmd.AddCommand(normalizeCmd)
}
