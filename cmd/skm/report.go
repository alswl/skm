package main

import (
	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

// printJSON encodes v as JSON to the command's stdout. It is the single
// stdout writer for --json commands, keeping stdout clean (FR-030).
func printJSON(cmd *cobra.Command, v any) error {
	return common.PrintJSON(cmd.OutOrStdout(), v)
}
