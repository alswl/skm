package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

var deployFlags struct {
	repo    string
	into    string
	noRepo  bool
	targets []string
	only    []string
}

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Create a personal skill library and link it to selected targets, or copy with --no-repo",
	Long: `Deploy directory skills from a local directory or provider address.

By default, create ./skm as a new personal library, import the selected skills,
then link them into explicitly selected targets. --into chooses another new
library directory; any existing destination, including an empty directory or
symlink, is refused even with --force.

--no-repo instead copies complete skills directly into the selected targets,
without a personal library or persistent cache. Existing target slots require
--force; upstream removals do not delete older copies. --into and --no-repo
are mutually exclusive. --target is required and can be repeated or comma-separated.
Target paths retain their configured meaning; they are not relocated to cwd.

Only directory skills and built-in skill-symlink target strategies are supported.
Local working directories are never pulled. --dry-run requires a local directory
and performs no writes. Explicit --root is rejected; use --into for a new library.
For an existing library, use import/install/update rather than repeating deploy.`,
	Example: "  skm deploy --repo ./source --target codex,claude-skills\n  skm deploy --repo ./source --into ./my-library --target codex\n  skm deploy --repo ./source --no-repo --target codex",
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return printDeployFailure(cmd, deployOptions(cmd), err)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := deployOptions(cmd)
		if err := services.ValidateDeployOptions(opts); err != nil {
			return printDeployFailure(cmd, opts, err)
		}
		if cmd.Flags().Changed("root") {
			return printDeployFailure(cmd, opts, fmt.Errorf("deploy: --root is unsupported; use --into for a new library"))
		}
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return printDeployFailure(cmd, opts, err)
		}
		result, deployErr := svc.Deploy(cmd.Context(), opts)
		if err = printDeployResult(cmd, result); err != nil {
			return err
		}
		return deployErr
	},
}

func deployOptions(cmd *cobra.Command) services.DeployOptions {
	return services.DeployOptions{Repo: deployFlags.repo, Into: deployFlags.into, IntoSet: cmd.Flags().Changed("into"), NoRepo: deployFlags.noRepo, Targets: deployFlags.targets, Only: deployFlags.only, Force: flagForce, DryRun: flagDryRun}
}

func printDeployFailure(cmd *cobra.Command, opts services.DeployOptions, err error) error {
	result := services.NewDeployResult(opts)
	_ = result.Fail("invalid_arguments", err)
	if outputErr := printDeployResult(cmd, result); outputErr != nil {
		return outputErr
	}
	return err
}

func printDeployResult(cmd *cobra.Command, r *services.DeployResult) error {
	if flagJSON {
		return printJSON(cmd, r)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "deploy %s: mode=%s phase=%s targets=%v\n", r.Repo, r.Mode, r.Phase, r.Targets)
	if r.Destination != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "repository: %s\n", *r.Destination)
	}
	for _, op := range r.Operations {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s -> %s %s\n", op.Status, op.Action, op.Skill, op.Path, op.Message)
	}
	for _, hint := range r.Recovery {
		fmt.Fprintln(cmd.OutOrStdout(), hint)
	}
	return nil
}

func init() {
	deployCmd.Flags().StringVar(&deployFlags.repo, "repo", "", "local skill directory or provider address (required)")
	deployCmd.Flags().StringVar(&deployFlags.into, "into", "", "new personal library directory (default ./skm)")
	deployCmd.Flags().BoolVar(&deployFlags.noRepo, "no-repo", false, "copy complete skills directly to targets without a personal library")
	deployCmd.Flags().StringSliceVar(&deployFlags.targets, "target", nil, "target name(s), comma-separated or repeated (required)")
	deployCmd.Flags().StringSliceVar(&deployFlags.only, "only", nil, "only deploy these skill names")
	rootCmd.AddCommand(deployCmd)
}
