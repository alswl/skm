package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

func init() {
	machine := &cobra.Command{
		Use: "machine", Short: "Manage machine skill installations",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if flagDryRun {
				switch cmd.CommandPath() {
				case "skm machine apply", "skm machine directory add", "skm machine directory path", "skm machine directory tag":
					return common.WithExitCode(fmt.Errorf("%s: --dry-run is not supported; use machine plan for a read-only preview", cmd.CommandPath()), common.ExitError)
				}
			}
			return nil
		},
	}
	rootCmd.AddCommand(machine)
	machine.AddCommand(newMachineDirectoryCommand(), newMachineScanCommand(), newMachinePlanCommand(), newMachineApplyCommand())
}
