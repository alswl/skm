package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Share selected local skill/command entries as a source-address PAYLOAD (never raw content)",
}

// shareProgress reports "N/M name" on stderr while collecting a multi-entry
// selection, so a large default-all `share create` isn't silent for minutes.
func shareProgress(cmd *cobra.Command) services.ShareProgressFunc {
	return func(done, total int, name string) {
		if total <= 1 {
			return
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "share: %d/%d %s\n", done, total, name)
	}
}

var shareCreateCmd = &cobra.Command{
	Use:   "create [NAME ...]",
	Short: "Create a share PAYLOAD naming each entry's source address (no file content)",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.CreateShare(cmd.Context(), args, shareProgress(cmd))
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, result)
		}
		for _, warning := range result.Warnings {
			fmt.Fprintln(cmd.ErrOrStderr(), "share:", warning)
		}
		fmt.Fprintln(cmd.OutOrStdout(), result.Command)
		return nil
	},
}

var shareApplyCmd = &cobra.Command{
	Use:   "apply PAYLOAD",
	Short: "Import and install every entry in an inline share PAYLOAD",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		result, err := svc.ApplyShare(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if flagJSON {
			if err := printJSON(cmd, result); err != nil {
				return err
			}
		} else {
			printShareApplyResult(cmd, result)
		}
		if !result.Success {
			return common.WithExitCode(fmt.Errorf("share apply: one or more items were skipped or failed"), common.ExitObject)
		}
		return nil
	},
}

func printShareApplyResult(cmd *cobra.Command, result *services.ShareApplyResult) {
	for _, item := range result.Items {
		switch item.Status {
		case "imported":
			fmt.Fprintf(cmd.OutOrStdout(), "imported %q, but %s\n", item.Name, item.Reason)
		case "skipped", "failed":
			fmt.Fprintf(cmd.OutOrStdout(), "%s %q: %s\n", item.Status, item.Name, item.Reason)
		default:
			fmt.Fprintf(cmd.OutOrStdout(), "%s %q (%s)\n", item.Status, item.Name, item.Kind)
			for _, report := range item.Results {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s -> %s\n", report.Target, report.Status)
			}
		}
	}
}

func init() {
	shareCmd.AddCommand(shareCreateCmd, shareApplyCmd)
	rootCmd.AddCommand(shareCmd)
}
