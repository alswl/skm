package builtins

// All returns the six built-in target declarations in their public order.
func All() []BuiltinTargetDefinition {
	return []BuiltinTargetDefinition{
		claudeSkills(), claudeCommands(), codex(), pi(), dsh(), agents(),
	}
}
