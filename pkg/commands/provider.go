package commands

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(providerCmd)
}

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage acquisition providers (built-in and plugin)",
}

func orDashStr(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
