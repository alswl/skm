package main

import (
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

var targetCmd = &cobra.Command{
	Use:   "target",
	Short: "Manage install targets (platform, path, accepted kinds, install strategy)",
}

var targetListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List configured targets",
	Example: "  skm target list --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		rep := svc.TargetList()
		if flagJSON {
			return printJSON(cmd, rep)
		}
		for _, t := range rep.Targets {
			fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-10s %-8s %s\n", t.Name, t.Platform, t.PathState, t.Path)
		}
		for _, inv := range rep.Invalid {
			fmt.Fprintf(cmd.OutOrStdout(), "%-16s INVALID: %s\n", "—", inv.Reason)
		}
		return nil
	},
}

var targetFlags struct {
	name       string
	platform   string
	path       string
	accepts    string
	strategies []string
}

func resetTargetFlags() {
	targetFlags.name, targetFlags.platform, targetFlags.path, targetFlags.accepts = "", "", "", ""
	targetFlags.strategies = nil
}

var targetAddCmd = &cobra.Command{
	Use:     "add",
	Short:   "Add a new target",
	Example: "  skm target add --name my-tool --platform mytool --path ~/.mytool/skills --accepts skill,command --strategy skill=skill-symlink --strategy command=command-adapter --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		t := common.InstallTarget{
			Name: targetFlags.name, Platform: targetFlags.platform, Path: targetFlags.path,
			Accepts: parseAccepts(targetFlags.accepts), Strategies: parseStrategies(targetFlags.strategies),
		}
		added, err := svc.TargetAdd(t)
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, map[string]any{"added": added, "success": true})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "added %s\n", added.Name)
		return nil
	},
}

var targetUpdateCmd = &cobra.Command{
	Use:     "update",
	Short:   "Update an existing target",
	Example: "  skm target update --name my-tool --path ~/.mytool/skills2 --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		updated, err := svc.TargetUpdate(targetFlags.name, func(t *common.InstallTarget) {
			if cmd.Flags().Changed("platform") {
				t.Platform = targetFlags.platform
			}
			if cmd.Flags().Changed("path") {
				t.Path = targetFlags.path
			}
			if cmd.Flags().Changed("accepts") {
				t.Accepts = parseAccepts(targetFlags.accepts)
			}
			if cmd.Flags().Changed("strategy") {
				t.Strategies = parseStrategies(targetFlags.strategies)
			}
		})
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, map[string]any{"updated": updated, "success": true})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", updated.Name)
		return nil
	},
}

var targetRemoveCmd = &cobra.Command{
	Use:     "remove",
	Short:   "Remove a target",
	Example: "  skm target remove --name my-tool --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		if err := svc.TargetRemove(targetFlags.name); err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, map[string]any{"removed": targetFlags.name, "success": true})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", targetFlags.name)
		return nil
	},
}

var targetValidateCmd = &cobra.Command{
	Use:     "validate [name]",
	Short:   "Validate targets, reporting a pass/fail reason for each",
	Example: "  skm target validate --json\n  skm target validate codex",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		rep := svc.TargetValidate(name)
		if flagJSON {
			if err := printJSON(cmd, rep); err != nil {
				return err
			}
		} else {
			for _, r := range rep.Results {
				if r.OK {
					fmt.Fprintf(cmd.OutOrStdout(), "%-16s ok (%s)\n", r.Name, r.PathState)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%-16s FAILED: %s\n", r.Name, *r.Error)
				}
			}
		}
		if !rep.Success {
			return common.WithExitCode(fmt.Errorf("target validate: one or more targets failed"), common.ExitObject)
		}
		return nil
	},
}

var targetPluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage Target plugins (out-of-process install strategies)",
}

var targetPluginListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List discovered Target plugins",
	Example: "  skm target plugin list --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		rep := svc.TargetPluginList()
		if flagJSON {
			return printJSON(cmd, rep)
		}
		for _, p := range rep.Plugins {
			if p.Loaded {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-8v %s\n", p.ID, p.Kinds, p.Description)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s FAILED: %s\n", orDashStr(p.ID), p.Error.Message)
			}
		}
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{targetAddCmd, targetUpdateCmd, targetRemoveCmd} {
		c.Flags().StringVar(&targetFlags.name, "name", "", "target name")
	}
	for _, c := range []*cobra.Command{targetAddCmd, targetUpdateCmd} {
		c.Flags().StringVar(&targetFlags.platform, "platform", "", "descriptive platform label")
		c.Flags().StringVar(&targetFlags.path, "path", "", "destination directory")
		c.Flags().StringVar(&targetFlags.accepts, "accepts", "", "comma-separated kinds: skill,command")
		c.Flags().StringArrayVar(&targetFlags.strategies, "strategy", nil, "kind=strategy, e.g. skill=skill-symlink or skill=plugin:<id> (repeatable)")
	}
	targetPluginCmd.AddCommand(targetPluginListCmd)
	targetCmd.AddCommand(targetListCmd, targetAddCmd, targetUpdateCmd, targetRemoveCmd, targetValidateCmd, targetPluginCmd)
	rootCmd.AddCommand(targetCmd)
}

// parseAccepts splits a comma-separated "skill,command" flag value.
func parseAccepts(s string) []common.EntryKind {
	if s == "" {
		return nil
	}
	var out []common.EntryKind
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, common.EntryKind(part))
		}
	}
	return out
}

// parseStrategies parses repeated "kind=strategy" flag values.
func parseStrategies(pairs []string) map[common.EntryKind]common.InstallStrategy {
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[common.EntryKind]common.InstallStrategy, len(pairs))
	for _, p := range pairs {
		k, v, found := strings.Cut(p, "=")
		if !found {
			continue
		}
		out[common.EntryKind(k)] = common.InstallStrategy(v)
	}
	return out
}
