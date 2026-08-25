package targets

import (
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
)

// Context keeps path resolution injectable so registry tests do not depend on
// process-global environment values.
type Context struct {
	Home   string
	Getenv func(string) string
}

// BuiltinDefinition is the declaration form of an install target.
type BuiltinDefinition struct {
	Name        string
	Platform    string
	ResolvePath func(Context) string
	Accepts     []common.EntryKind
	Strategies  map[common.EntryKind]common.InstallStrategy
	NameRule    string
}

// Materialize resolves a definition into an independent runtime record.
func (d BuiltinDefinition) Materialize(ctx Context) common.InstallTarget {
	accepts := append([]common.EntryKind(nil), d.Accepts...)
	strategies := make(map[common.EntryKind]common.InstallStrategy, len(d.Strategies))
	for kind, strategy := range d.Strategies {
		strategies[kind] = strategy
	}
	path := ""
	if d.ResolvePath != nil {
		path = d.ResolvePath(ctx)
	}
	return common.InstallTarget{
		Name: d.Name, Platform: d.Platform, Path: path, Builtin: true,
		Accepts: accepts, Strategies: strategies, NameRule: d.NameRule,
	}
}

func skillTarget(name, platform string, resolve func(Context) string, nameRule string) BuiltinDefinition {
	return BuiltinDefinition{
		Name: name, Platform: platform, ResolvePath: resolve, NameRule: nameRule,
		Accepts:    []common.EntryKind{common.KindSkill},
		Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink},
	}
}

func homePath(ctx Context, envName, fallback string, suffix ...string) string {
	home := filepath.Join(ctx.Home, fallback)
	if ctx.Getenv != nil {
		if value := ctx.Getenv(envName); value != "" {
			home = value
		}
	}
	return filepath.Join(append([]string{home}, suffix...)...)
}
