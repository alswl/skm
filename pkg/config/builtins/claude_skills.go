package builtins

import "path/filepath"

func claudeSkills() BuiltinTargetDefinition {
	return skillTarget("claude-skills", "claude", func(ctx Context) string {
		return filepath.Join(ctx.Home, ".claude", "skills")
	}, "")
}
