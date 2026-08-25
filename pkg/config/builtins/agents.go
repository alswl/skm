package builtins

import "github.com/alswl/skm/skm/pkg/common"

func agents() BuiltinTargetDefinition {
	return skillTarget("agents", "agents", func(ctx Context) string {
		return homePath(ctx, "DSH_AGENTS_HOME", ".agents", "skills")
	}, common.NameRuleKebabCase)
}
