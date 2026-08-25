package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

var targetValidateCmd = &cobra.Command{
	Use:     "validate [name]",
	Short:   "Validate targets, reporting a pass/fail reason for each",
	Example: "  skm target validate --json\n  skm target validate codex",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		rep := svc.TargetValidate(name)
		if flagJSON {
			if err := printJSON(cmd, rep); err != nil {
				return err
			}
		} else {
			for _, r := range rep.Results {
				if r.OK {
					fmt.Fprintf(cmd.OutOrStdout(), "%-16s ok (%s)\n", r.Name, r.PathState)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%-16s FAILED: %s\n", r.Name, *r.Error)
				}
			}
		}
		if !rep.Success {
			return common.WithExitCode(fmt.Errorf("target validate: one or more targets failed"), common.ExitObject)
		}
		return nil
	},
}

func init() {
	targetCmd.AddCommand(targetValidateCmd)
}
