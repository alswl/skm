package commands

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

func newLifecycleCommand(use, short string, run func(ctx context.Context, svc *services.Services, name string, opts services.LifecycleOptions) (*services.LifecycleResult, error)) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := servicesFor(cmd)
			if err != nil {
				return err
			}
			result, err := run(cmd.Context(), svc, args[0], services.LifecycleOptions{Force: flagForce, DryRun: flagDryRun})
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd, result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %q -> %s\n", result.Action, result.Name, result.Path)
			return nil
		},
	}
}

var archiveCmd = newLifecycleCommand("archive NAME", "Move an active entry into the archive",
	func(ctx context.Context, s *services.Services, name string, o services.LifecycleOptions) (*services.LifecycleResult, error) {
		return s.Archive(ctx, name, o)
	})
var unarchiveCmd = newLifecycleCommand("unarchive NAME", "Restore an archived entry",
	func(ctx context.Context, s *services.Services, name string, o services.LifecycleOptions) (*services.LifecycleResult, error) {
		return s.Unarchive(ctx, name, o)
	})
var deleteCmd = newLifecycleCommand("delete NAME", "Permanently remove an entry (requires --force)",
	func(ctx context.Context, s *services.Services, name string, o services.LifecycleOptions) (*services.LifecycleResult, error) {
		return s.Delete(ctx, name, o)
	})

var normalizeFlags struct {
	provider string
}

// normalizeCmd exposes the same relocation operation as the TUI's “move to
// standard location” action.  Keeping it separate from the generic lifecycle
// constructor makes the destination provider explicit and scriptable.
var normalizeCmd = &cobra.Command{
	Use:     "normalize NAME",
	Short:   "Move a non-standard entry into a provider location",
	Example: "  skm normalize review --provider local --dry-run\n  skm normalize review --provider github --json",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.Normalize(cmd.Context(), args[0], normalizeFlags.provider,
			services.LifecycleOptions{DryRun: flagDryRun})
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		verb := "normalized"
		if result.DryRun {
			verb = "would normalize"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %q -> %s\n", verb, result.Name, result.Path)
		return nil
	},
}
var toCommandCmd = newLifecycleCommand("to-command NAME", "Convert a directory skill into a command",
	func(ctx context.Context, s *services.Services, name string, o services.LifecycleOptions) (*services.LifecycleResult, error) {
		return s.Convert(ctx, name, common.KindCommand, o)
	})
var toSkillCmd = newLifecycleCommand("to-skill NAME", "Convert a directory command into a skill",
	func(ctx context.Context, s *services.Services, name string, o services.LifecycleOptions) (*services.LifecycleResult, error) {
		return s.Convert(ctx, name, common.KindSkill, o)
	})

func init() {
	normalizeCmd.Flags().StringVar(&normalizeFlags.provider, "provider", "local", "destination provider id")
	rootCmd.AddCommand(archiveCmd, unarchiveCmd, deleteCmd, normalizeCmd, toCommandCmd, toSkillCmd)
}
