package targets

import "github.com/alswl/skm/skm/pkg/common"

// BuiltinDefinitions returns the six built-in target declarations in their
// public order. Each declares its own accepts/strategies (FR-012/FR-013,
// data-model.md). Codex receives skills as directory links and commands
// through command-adapter, whose wrapper directory contains a regular
// SKILL.md file. pi has no separate commands concept — it can auto-register a
// skill as a /skill:name command itself — so it only accepts skill, via
// skill-symlink into its ~/.pi/agent/skills convention. The deepseek-harness
// built-ins (dsh, agents) cover dsh's two user-level skill roots, its own and
// the shared cross-agent directory, skills-only via skill-symlink with a
// kebab-case name rule (006-deepseek-harness-target).
//
// skm ships built-ins only for these widely-used public tools: any other tool,
// private or public, is added with `skm target add`, not a hardcoded default.
func BuiltinDefinitions() []BuiltinDefinition {
	return []BuiltinDefinition{
		claudeSkills(), claudeCommands(), codex(), pi(), dsh(), agents(),
	}
}

// Builtins materializes every built-in target against ctx. Path resolution is
// injected rather than read from the process environment so callers (and
// tests) decide what $HOME and the DSH_* overrides are.
func Builtins(ctx Context) []common.InstallTarget {
	defs := BuiltinDefinitions()
	out := make([]common.InstallTarget, 0, len(defs))
	for _, definition := range defs {
		out = append(out, definition.Materialize(ctx))
	}
	return out
}
