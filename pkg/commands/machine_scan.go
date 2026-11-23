package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

func newMachineScanCommand() *cobra.Command {
	var scanDirectory, scanPath, scanSkill string
	var scanUser bool
	scan := &cobra.Command{
		Use: "scan", Short: "Scan a registered directory's skills", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scopes := 0
			if scanDirectory != "" {
				scopes++
			}
			if scanPath != "" {
				scopes++
			}
			if scanUser {
				scopes++
			}
			if scopes > 1 {
				return fmt.Errorf("select at most one of --directory, --path or --user")
			}
			svc, err := servicesFor(cmd)
			if err != nil {
				return err
			}
			var items []common.InstallationObservation
			if scanUser {
				items, err = svc.ScanMachineUser()
			} else if scanDirectory != "" {
				items, err = svc.ScanMachineDirectory(scanDirectory)
			} else if scanPath != "" {
				items, err = svc.ScanMachinePath(scanPath)
			} else {
				items, err = svc.ScanMachineAll()
			}
			if err != nil {
				return err
			}
			if scanSkill != "" {
				items, err = svc.FilterMachineSkill(items, scanSkill)
				if err != nil {
					return err
				}
			}
			if flagJSON {
				diagnostics := []common.InstallationObservation{}
				for _, item := range items {
					if item.Status == "error" || item.Status == "recovery" {
						diagnostics = append(diagnostics, item)
					}
				}
				return printJSON(cmd, map[string]any{"items": items, "diagnostics": diagnostics})
			}
			for _, item := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s", item.Target, item.Status, item.Path)
				if item.Error != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "\t%s", item.Error)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}
	scan.Flags().StringVar(&scanDirectory, "directory", "", "registered directory ID")
	scan.Flags().BoolVar(&scanUser, "user", false, "scan user-level targets")
	scan.Flags().StringVar(&scanPath, "path", "", "unregistered Git root")
	scan.Flags().StringVar(&scanSkill, "skill", "", "filter by library skill")
	return scan

}
