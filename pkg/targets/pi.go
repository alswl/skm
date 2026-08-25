package targets

import "path/filepath"

func pi() BuiltinDefinition {
	return skillTarget("pi", "pi", func(ctx Context) string {
		return filepath.Join(ctx.Home, ".pi", "agent", "skills")
	}, "")
}
