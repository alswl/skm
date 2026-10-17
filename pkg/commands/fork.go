package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

var forkFlags struct{ replace, discard bool }

var forkCmd = &cobra.Command{
	Use:   "fork NAME",
	Short: "Give a Skill its own editable copy in skills-fork/",
	Long: "Copy a Skill into skills-fork/ so you can edit it. While the copy exists it is the Skill's active content.\n\n" +
		"`skm update` then carries your changes onto each new upstream version with a three-way merge (`git merge-file`), and stops without changing anything when they conflict.\n" +
		"--replace copies the upstream again, dropping the old copy. --discard deletes the copy and returns to the upstream.",
	Example: "  skm fork review\n  skm fork review --discard",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.Fork(cmd.Context(), args[0], services.ForkOptions{Replace: forkFlags.replace, Discard: forkFlags.discard})
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %q -> %s\n", result.Action, result.Name, result.ActivePath)
		return nil
	},
}

func init() {
	forkCmd.Flags().BoolVar(&forkFlags.replace, "replace", false, "discard the existing fork and copy the upstream again")
	forkCmd.Flags().BoolVar(&forkFlags.discard, "discard", false, "delete the fork and return to the upstream")
	rootCmd.AddCommand(forkCmd)
}
