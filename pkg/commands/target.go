package commands

import (
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/spf13/cobra"
)

var targetCmd = &cobra.Command{
	Use:   "target",
	Short: "Manage install targets (platform, path, accepted kinds, install strategy)",
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

// targetFlagsFor registers the flags that describe a target's shape, shared by
// `target add` and `target update`.
func targetFlagsFor(c *cobra.Command) {
	c.Flags().StringVar(&targetFlags.name, "name", "", "target name")
	c.Flags().StringVar(&targetFlags.platform, "platform", "", "descriptive platform label")
	c.Flags().StringVar(&targetFlags.path, "path", "", "destination directory")
	c.Flags().StringVar(&targetFlags.accepts, "accepts", "", "comma-separated kinds: skill,command")
	c.Flags().StringArrayVar(&targetFlags.strategies, "strategy", nil, "kind=strategy, e.g. skill=skill-symlink or skill=plugin:<id> (repeatable)")
}

func init() {
	rootCmd.AddCommand(targetCmd)
}

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
