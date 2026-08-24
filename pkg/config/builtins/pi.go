package builtins

import "path/filepath"

func pi() BuiltinTargetDefinition {
	return skillTarget("pi", "pi", func(ctx Context) string {
		return filepath.Join(ctx.Home, ".pi", "agent", "skills")
	}, "")
}
