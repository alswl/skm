package commands

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

func newLifecycleCommand(use, short string, run func(ctx context.Context, svc *services.Services, name string, opts engines.LifecycleOptions) (*services.LifecycleResult, error)) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := servicesFor(cmd)
			if err != nil {
				return err
			}
			result, err := run(cmd.Context(), svc, args[0], engines.LifecycleOptions{Force: flagForce, DryRun: flagDryRun})
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
