package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage acquisition providers (built-in and plugin)",
}

var providerListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List providers in resolution order",
	Example: "  skm provider list --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		rep := svc.ProviderList()
		if flagJSON {
			return printJSON(cmd, rep)
		}
		for _, p := range rep.Providers {
			if p.Loaded {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-8s %s\n", p.ID, p.Kind, p.Description)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-8s FAILED: %s\n", orDashStr(p.ID), p.Kind, p.Error.Message)
			}
		}
		return nil
	},
}

var providerValidateCmd = &cobra.Command{
	Use:     "validate [id]",
	Short:   "Validate providers, reporting a pass/fail reason for each",
	Example: "  skm provider validate --json\n  skm provider validate github",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		id := ""
		if len(args) == 1 {
			id = args[0]
		}
		rep := svc.ProviderValidate(id)
		if flagJSON {
			if err := printJSON(cmd, rep); err != nil {
				return err
			}
		} else {
			for _, r := range rep.Results {
				if r.OK {
					fmt.Fprintf(cmd.OutOrStdout(), "%-14s ok\n", r.ID)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%-14s FAILED: %s\n", orDashStr(r.ID), r.Error.Message)
				}
			}
		}
		if !rep.Success {
			return common.WithExitCode(fmt.Errorf("provider validate: one or more providers failed"), common.ExitObject)
		}
		return nil
	},
}

func init() {
	providerCmd.AddCommand(providerListCmd, providerValidateCmd)
	rootCmd.AddCommand(providerCmd)
}

func orDashStr(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
