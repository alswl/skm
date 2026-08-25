package targets

import "github.com/alswl/skm/skm/pkg/common"

func dsh() BuiltinDefinition {
	return skillTarget("dsh", "dsh", func(ctx Context) string {
		return homePath(ctx, "DSH_HOME", ".dsh", "skills")
	}, common.NameRuleKebabCase)
}
