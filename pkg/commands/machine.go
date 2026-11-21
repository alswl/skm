package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

func init() {
	directory := newMachineDirectoryCommand()
	directory.PersistentPreRunE = rejectInstallationDryRun
	apply := newMachineApplyCommand()
	apply.PreRunE = rejectInstallationDryRun
	rootCmd.AddCommand(directory, newMachineScanCommand(), newMachinePlanCommand(), apply)
}

func rejectInstallationDryRun(cmd *cobra.Command, _ []string) error {
	if flagDryRun {
		switch cmd.CommandPath() {
		case "skm apply", "skm directory add", "skm directory path", "skm directory tag":
			return common.WithExitCode(fmt.Errorf("%s: --dry-run is not supported; use skm plan for a read-only preview", cmd.CommandPath()), common.ExitError)
		}
	}
	return nil
}
