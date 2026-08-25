package builtins

import "github.com/alswl/skm/skm/pkg/common"

func dsh() BuiltinTargetDefinition {
	return skillTarget("dsh", "dsh", func(ctx Context) string {
		return homePath(ctx, "DSH_HOME", ".dsh", "skills")
	}, common.NameRuleKebabCase)
}
