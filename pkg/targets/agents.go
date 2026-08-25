package targets

import "github.com/alswl/skm/skm/pkg/common"

func agents() BuiltinDefinition {
	return skillTarget("agents", "agents", func(ctx Context) string {
		return homePath(ctx, "DSH_AGENTS_HOME", ".agents", "skills")
	}, common.NameRuleKebabCase)
}
