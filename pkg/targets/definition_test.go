package targets

import (
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestMaterializeCopiesCollections(t *testing.T) {
	d := skillTarget("demo", "demo", func(Context) string { return "/tmp/demo" }, "kebab-case")
	one := d.Materialize(Context{})
	two := d.Materialize(Context{})
	one.Accepts[0] = common.KindCommand
	one.Strategies[common.KindSkill] = common.StrategyCommandMarker
	require.Equal(t, common.KindSkill, two.Accepts[0])
	require.Equal(t, common.StrategySkillSymlink, two.Strategies[common.KindSkill])
	require.True(t, two.Builtin)
}

func TestDshAndAgentsResolveOverrides(t *testing.T) {
	ctx := Context{Home: "/home/user", Getenv: func(key string) string {
		if key == "DSH_HOME" {
			return "/tmp/dsh"
		}
		if key == "DSH_AGENTS_HOME" {
			return "/tmp/agents"
		}
		return ""
	}}
	byName := map[string]string{}
	for _, d := range BuiltinDefinitions() {
		byName[d.Name] = d.Materialize(ctx).Path
	}
	require.Equal(t, "/tmp/dsh/skills", byName["dsh"])
	require.Equal(t, "/tmp/agents/skills", byName["agents"])
}
