package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage installed provider/target plugins",
	Long: `Install a local plugin executable into ~/.config/skm/plugins, list what is
installed there, and remove one again — instead of symlinking by hand.

A plugin is linked, not copied, so edits in your own checkout take effect
immediately.`,
}

var pluginAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Link a local plugin executable into the plugin directory",
	Example: `  skm plugin add ~/ws/skills/skm/plugins/providers/ali-skills
  skm plugin add ~/bin/my-target --kind target --name codefuse`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		info, err := svc.PluginAdd(args[0], pluginKind, pluginName, flagForce)
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, map[string]any{"added": info, "success": true})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "added %s %s -> %s\n", info.Kind, info.Path, info.Source)
		return nil
	},
}

var pluginListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List plugins installed in the plugin directory",
	Example: "  skm plugin list --json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		rep := svc.PluginList()
		if flagJSON {
			return printJSON(cmd, rep)
		}
		for _, p := range rep.Plugins {
			state := ""
			if p.Broken {
				state = "  (broken)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-10s %-16s %s%s\n", p.Kind, p.Name, orDashStr(p.Source), state)
		}
		return nil
	},
}

var pluginRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Short:   "Remove a plugin from the plugin directory",
	Example: "  skm plugin remove ali-skills --kind provider",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := deployServicesFor(cmd)
		if err != nil {
			return err
		}
		info, err := svc.PluginRemove(args[0], pluginKind, flagForce)
		if err != nil {
			return err
		}
		if flagJSON {
			return printJSON(cmd, map[string]any{"removed": info, "success": true})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "removed %s %s\n", info.Kind, info.Path)
		return nil
	},
}

var (
	pluginKind string
	pluginName string
)

func init() {
	pluginAddCmd.Flags().StringVar(&pluginKind, "kind", "", "provider or target (inferred from a providers/ or targets/ source directory)")
	pluginAddCmd.Flags().StringVar(&pluginName, "name", "", "name to install under (default: the file's name)")
	pluginRemoveCmd.Flags().StringVar(&pluginKind, "kind", "", "provider or target (needed only when both kinds share the name)")
	pluginCmd.AddCommand(pluginAddCmd, pluginListCmd, pluginRemoveCmd)
	rootCmd.AddCommand(pluginCmd)
}
