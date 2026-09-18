package commands

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

var shareYes bool

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Create and install compact skill share PAYLOADs",
}

var shareCreateCmd = &cobra.Command{
	Use:   "create [NAME ...]",
	Short: "Create a complete share install command",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.CreateShare(cmd.Context(), args)
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		fmt.Fprintln(cmd.OutOrStdout(), result.Command)
		return nil
	},
}

var shareInstallCmd = &cobra.Command{
	Use:   "install PAYLOAD",
	Short: "Preview or install an inline share PAYLOAD",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		preview, err := svc.PreviewShare(args[0])
		if err != nil {
			return err
		}
		if flagJSON && !shareYes {
			return common.WithExitCode(fmt.Errorf("share install: --json requires --yes"), common.ExitError)
		}
		if !flagJSON && !shareYes {
			printSharePreview(cmd, preview)
			ok, confirmErr := confirmShare(cmd)
			if confirmErr != nil {
				return confirmErr
			}
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "cancelled")
				return nil
			}
		}
		result, err := svc.InstallShare(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if flagJSON {
			if err := printJSON(cmd, result); err != nil {
				return err
			}
		} else {
			printShareResult(cmd, result)
		}
		if !result.Success {
			return common.WithExitCode(fmt.Errorf("share install: one or more items failed"), common.ExitObject)
		}
		return nil
	},
}

func printSharePreview(cmd *cobra.Command, preview *services.SharePreview) {
	fmt.Fprintf(cmd.OutOrStdout(), "share PAYLOAD: %d item(s), all compatible Targets by default\n", len(preview.Items))
	for _, item := range preview.Items {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s (%s) <- %s\n", item.Name, item.Kind, item.Source)
	}
}

func confirmShare(cmd *cobra.Command) (bool, error) {
	fmt.Fprint(cmd.OutOrStdout(), "install all items? [y/N] ")
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, nil
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}

func printShareResult(cmd *cobra.Command, result *services.ShareInstallResult) {
	for _, item := range result.Items {
		if item.Reason != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %q: %s (%s)\n", item.Status, item.Name, item.Reason, item.Source)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %q (%s)\n", item.Status, item.Name, item.Kind)
		for _, report := range item.Results {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s -> %s\n", report.Target, report.Status)
		}
	}
}

func init() {
	shareInstallCmd.Flags().BoolVar(&shareYes, "yes", false, "install directly without confirmation")
	shareCmd.AddCommand(shareCreateCmd, shareInstallCmd)
	rootCmd.AddCommand(shareCmd)
}
