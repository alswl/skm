package targets

import "path/filepath"

func claudeSkills() BuiltinDefinition {
	return skillTarget("claude-skills", "claude", func(ctx Context) string {
		return filepath.Join(ctx.Home, ".claude", "skills")
	}, "")
}
