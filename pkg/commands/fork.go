package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/spf13/cobra"
)

var forkCmd = &cobra.Command{
	Use: "fork NAME", Short: "Create a complete local customization in skills-fork and switch existing installs",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		upstream, _ := cmd.Flags().GetBool("upstream")
		replace, _ := cmd.Flags().GetBool("replace")
		res, err := svc.Fork(cmd.Context(), args[0], engines.SyncOptions{Upstream: upstream, Replace: replace, DryRun: flagDryRun})
		return printSyncResult(cmd, res, err)
	},
}

func init() {
	forkCmd.Flags().Bool("upstream", false, "use community content and retain the inactive fork")
	forkCmd.Flags().Bool("replace", false, "explicitly delete and recreate an existing fork")
	forkCmd.MarkFlagsMutuallyExclusive("upstream", "replace")
	rootCmd.AddCommand(forkCmd)
}
func printSyncResult(cmd *cobra.Command, res *engines.UpdateResult, err error) error {
	if res == nil {
		res = &engines.UpdateResult{Outcome: "failed"}
		if err != nil {
			res.Reason = err.Error()
		}
	}
	if flagJSON {
		if outputErr := printJSON(cmd, res); outputErr != nil {
			return outputErr
		}
	} else {
		outcome := res.Outcome
		if outcome == "" {
			outcome = "current"
			if res.Changed {
				outcome = "updated"
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s (accepted=%v) %s\n", outcome, res.Accepted, res.ActivePath)
		if res.AttemptPath != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Manual update workspace: %s\n", res.AttemptPath)
		}
		if res.Reason != "" {
			fmt.Fprintln(cmd.OutOrStdout(), res.Reason)
		}
	}
	return err
}
