package builtins

import (
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestDshDefinitionDefaultsAndRule(t *testing.T) {
	target := dsh().Materialize(Context{Home: "/home/user", Getenv: func(string) string { return "" }})
	require.Equal(t, "/home/user/.dsh/skills", target.Path)
	require.Equal(t, "kebab-case", target.NameRule)
	require.Equal(t, []common.EntryKind{common.KindSkill}, target.Accepts)
	require.Equal(t, common.StrategySkillSymlink, target.Strategies[common.KindSkill])
}
