package commands

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{Use: "sync", Short: "Inspect customization and restore upstream tracking"}

func init() {
	diff := &cobra.Command{Use: "diff NAME", Short: "Show current content relative to its upstream baseline", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		text, err := s.SyncDiff(cmd.Context(), args[0])
		if err != nil {
			if flagJSON {
				if printErr := printJSON(cmd, map[string]any{"success": false, "reason": err.Error()}); printErr != nil {
					return printErr
				}
			}
			return err
		}
		if flagJSON {
			return printJSON(cmd, map[string]string{"diff": text})
		}
		fmt.Fprint(cmd.OutOrStdout(), text)
		return nil
	}}
	recoverCmd := &cobra.Command{Use: "recover", Short: "Recover an interrupted repository transaction", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		err = s.SyncRecover(cmd.Context(), flagDryRun)
		if flagJSON {
			reason := ""
			if err != nil {
				reason = err.Error()
			}
			if e := printJSON(cmd, map[string]any{"success": err == nil, "reason": reason}); e != nil {
				return e
			}
		}
		return err
	}}
	baseline := &cobra.Command{Use: "baseline NAME", Short: "Preview or accept an explicit historical baseline or new comparison point", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, err := servicesFor(cmd)
		if err != nil {
			return err
		}
		str := func(n string) string { v, _ := cmd.Flags().GetString(n); return v }
		yes := func(n string) bool { v, _ := cmd.Flags().GetBool(n); return v }
		res, err := s.SyncBaseline(cmd.Context(), args[0], services.BaselineOptions{Source: str("from"), Revision: str("revision"), Provider: str("provider"), Subpath: str("subpath"), ExpectedTree: str("expected-tree"), ExpectedLocal: str("expected-local"), NewStart: yes("new-start"), Accept: yes("accept"), DryRun: flagDryRun})
		if res == nil && err != nil && flagJSON {
			if printErr := printJSON(cmd, map[string]any{"success": false, "reason": err.Error()}); printErr != nil {
				return printErr
			}
		}
		if res != nil {
			if flagJSON {
				if e := printJSON(cmd, res); e != nil {
					return e
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s revision=%s tree=%s local=%s accepted=%v\n", res.Mode, res.Revision, res.Tree, res.Local, res.Accepted)
			}
		}
		return err
	}}
	for _, name := range []string{"from", "revision", "provider", "subpath", "expected-tree", "expected-local"} {
		baseline.Flags().String(name, "", name)
	}
	baseline.Flags().Bool("new-start", false, "explicitly use this revision as a new comparison point, not recovered history")
	baseline.Flags().Bool("accept", false, "accept the preview bound to the expected tree and local fingerprints")
	syncCmd.AddCommand(diff, recoverCmd, baseline)
	rootCmd.AddCommand(syncCmd)
}
