package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var targetRemoveCmd = &cobra.Command{
	Use:     "remove",
	Short:   "Remove a target",
	Example: "  skm target remove --name my-tool --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		if err := svc.TargetRemove(targetFlags.name); err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, map[string]any{"removed": targetFlags.name, "success": true})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", targetFlags.name)
		return nil
	},
}

func init() {
	targetRemoveCmd.Flags().StringVar(&targetFlags.name, "name", "", "target name")
	targetCmd.AddCommand(targetRemoveCmd)
}
