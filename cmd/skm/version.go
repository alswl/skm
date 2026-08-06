package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alswl/skm/skm/pkg/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("skm %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
