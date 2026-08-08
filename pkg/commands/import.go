package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

var importFlags struct {
	provider string
	kind     string
}

var importCmd = &cobra.Command{
	Use:     "import SOURCE",
	Short:   "Import a skill or command from a local path or provider address",
	Example: "  skm import ./my-skill --kind skill --force\n  skm import git@github.com:org/repo.git --json",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.Import(cmd.Context(), args[0], services.ImportOptions{
			Provider: importFlags.provider,
			Kind:     importFlags.kind,
			Force:    flagForce,
			DryRun:   flagDryRun,
		})
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "imported %s (%s) via %s -> %s\n", result.Name, result.Type, result.Provider, result.Path)
		return nil
	},
}

func init() {
	importCmd.Flags().StringVar(&importFlags.provider, "provider", "", "provider id for remote imports")
	importCmd.Flags().StringVar(&importFlags.kind, "kind", "auto", "kind hint: auto|skill|command")
	rootCmd.AddCommand(importCmd)
}
