package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

var targetAddCmd = &cobra.Command{
	Use:     "add",
	Short:   "Add a new target",
	Example: "  skm target add --name my-tool --platform mytool --path ~/.mytool/skills --accepts skill,command --strategy skill=skill-symlink --strategy command=command-adapter --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		t := common.InstallTarget{
			Name: targetFlags.name, Platform: targetFlags.platform, Path: targetFlags.path,
			Accepts: parseAccepts(targetFlags.accepts), Strategies: parseStrategies(targetFlags.strategies),
		}
		added, err := svc.TargetAdd(t)
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, map[string]any{"added": added, "success": true})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "added %s\n", added.Name)
		return nil
	},
}

func init() {
	targetFlagsFor(targetAddCmd)
	targetCmd.AddCommand(targetAddCmd)
}
