package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alswl/skm/skm/pkg/version"
)

// versionLine is the single rendering of build metadata, shared by the
// `version` subcommand and the --version flag so the two cannot drift.
func versionLine() string {
	return fmt.Sprintf("skm %s (commit %s, built %s)", version.Version, version.Commit, version.BuildDate())
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), versionLine())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	// Cobra only renders --version when Version is non-empty.
	rootCmd.Version = version.Version
	rootCmd.SetVersionTemplate("{{.Root.Annotations.versionLine}}\n")
	rootCmd.Annotations = map[string]string{"versionLine": versionLine()}
}
