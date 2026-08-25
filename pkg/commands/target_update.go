package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

var targetUpdateCmd = &cobra.Command{
	Use:     "update",
	Short:   "Update an existing target",
	Example: "  skm target update --name my-tool --path ~/.mytool/skills2 --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		updated, err := svc.TargetUpdate(targetFlags.name, func(t *common.InstallTarget) {
			if cmd.Flags().Changed("platform") {
				t.Platform = targetFlags.platform
			}
			if cmd.Flags().Changed("path") {
				t.Path = targetFlags.path
			}
			if cmd.Flags().Changed("accepts") {
				t.Accepts = parseAccepts(targetFlags.accepts)
			}
			if cmd.Flags().Changed("strategy") {
				t.Strategies = parseStrategies(targetFlags.strategies)
			}
		})
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, map[string]any{"updated": updated, "success": true})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", updated.Name)
		return nil
	},
}

func init() {
	targetFlagsFor(targetUpdateCmd)
	targetCmd.AddCommand(targetUpdateCmd)
}
