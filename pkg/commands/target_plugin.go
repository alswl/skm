package commands

import (
	"github.com/spf13/cobra"
)

var targetPluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage Target plugins (out-of-process install strategies)",
}

func init() {
	targetCmd.AddCommand(targetPluginCmd)
}
