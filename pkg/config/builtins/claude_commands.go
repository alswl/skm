package builtins

import (
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
)

func claudeCommands() BuiltinTargetDefinition {
	return BuiltinTargetDefinition{
		Name: "claude-commands", Platform: "claude",
		ResolvePath: func(ctx Context) string { return filepath.Join(ctx.Home, ".claude", "commands") },
		Accepts:     []common.EntryKind{common.KindCommand},
		Strategies:  map[common.EntryKind]common.InstallStrategy{common.KindCommand: common.StrategyCommandMarker},
	}
}
