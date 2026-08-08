package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/managers"
	"github.com/spf13/cobra"
)

var deployFlags struct {
	repo    string
	targets []string
	only    []string
}

var deployCmd = &cobra.Command{
	Use:     "deploy",
	Short:   "Clone/pull a repository and batch-install selected assets",
	Example: "  skm deploy --repo git@host:team/skills.git --target codex --only review,release",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if deployFlags.repo == "" {
			return fmt.Errorf("deploy: --repo is required")
		}
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.Deploy(cmd.Context(), managers.DeployOptions{
			Repo:    deployFlags.repo,
			Targets: deployFlags.targets,
			Only:    deployFlags.only,
			Force:   flagForce,
			DryRun:  flagDryRun,
		})
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "deploy %s (%s): %d skill(s), %d install(s)\n",
			result.Repo, result.Clone, len(result.Skills), len(result.Results))
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:     "export",
	Short:   "Emit a quote-safe skm deploy command for installed assets",
	Example: "  skm export",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.Export()
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		if result.Command == "" {
			return nil // nothing installed: no unscoped command (SC-008)
		}
		fmt.Fprintln(cmd.OutOrStdout(), result.Command)
		return nil
	},
}

func init() {
	deployCmd.Flags().StringVar(&deployFlags.repo, "repo", "", "git URL, local git repo, or plain directory")
	deployCmd.Flags().StringSliceVar(&deployFlags.targets, "target", nil, "target name(s); default all")
	deployCmd.Flags().StringSliceVar(&deployFlags.only, "only", nil, "only install these entry names")
	rootCmd.AddCommand(deployCmd, exportCmd)
}
