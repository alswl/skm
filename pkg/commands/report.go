package commands

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// printJSON encodes v as a single JSON object to the command's stdout,
// followed by a newline. Struct field order is preserved, which keeps golden
// fixtures deterministic (SC-002). It is the single stdout writer for --json
// commands, keeping stdout clean (FR-030).
func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
