package targets

import (
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
)

func codex() BuiltinDefinition {
	return BuiltinDefinition{
		Name: "codex", Platform: "codex",
		ResolvePath: func(ctx Context) string { return filepath.Join(ctx.Home, ".codex", "skills") },
		Accepts:     []common.EntryKind{common.KindSkill, common.KindCommand},
		Strategies: map[common.EntryKind]common.InstallStrategy{
			common.KindSkill: common.StrategySkillSymlink, common.KindCommand: common.StrategyCommandAdapter,
		},
	}
}
