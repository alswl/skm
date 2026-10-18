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
	Short: "Import skills into an initialized repository and link them to selected targets",
	Long: `Deploy directory skills from a local directory or provider address.

First initialize a personal library with skm init [DIRECTORY]. By default,
deploy uses the configured repository or discovers it upward from cwd; --root
selects an initialized library explicitly. Selected skills are imported into
that library, then linked into the selected targets. Existing skills and
target slots require --force to replace; unrelated content is retained.

--into instead creates a new library. Any existing --into destination,
including an empty directory or symlink, is refused even with --force.

--no-repo instead copies complete skills directly into the selected targets,
without a personal library or persistent cache. Existing target slots require
--force; upstream removals do not delete older copies. --into and --no-repo
are mutually exclusive. --root cannot be combined with --into or --no-repo.
--target is required and can be repeated or comma-separated.
Target paths retain their configured meaning; they are not relocated to cwd.
--target accepts target names or platform names such as claude. Platforms select
the skill target; ambiguous platforms require an explicit target name.

Only directory skills and built-in skill-symlink target strategies are supported.
Local working directories are never pulled. --dry-run requires a local directory
and performs no writes.`,
	Example: "  skm init ./my-library\n  skm deploy --repo ./source --root ./my-library --target codex,claude\n  skm deploy --repo ./source --into ./new-library --target codex\n  skm deploy --repo ./source --no-repo --target codex",
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
		if cmd.Flags().Changed("root") && (opts.IntoSet || opts.NoRepo) {
			return printDeployFailure(cmd, opts, fmt.Errorf("deploy: --root cannot be combined with --into or --no-repo"))
		}
		var svc *services.Services
		var err error
		if !opts.NoRepo && !opts.IntoSet {
			svc, err = servicesFor(cmd)
		} else {
			svc, err = deployServicesFor(cmd)
		}
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
	deployCmd.Flags().StringVar(&deployFlags.into, "into", "", "create a new personal library instead of using the initialized repository")
	deployCmd.Flags().BoolVar(&deployFlags.noRepo, "no-repo", false, "copy complete skills directly to targets without a personal library")
	deployCmd.Flags().StringSliceVar(&deployFlags.targets, "target", nil, "target or platform name(s), comma-separated or repeated (required)")
	deployCmd.Flags().StringSliceVar(&deployFlags.only, "only", nil, "only deploy these skill names")
	rootCmd.AddCommand(deployCmd)
}
