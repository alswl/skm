package builtins

func dsh() BuiltinTargetDefinition {
	return skillTarget("dsh", "dsh", func(ctx Context) string {
		return homePath(ctx, "DSH_HOME", ".dsh", "skills")
	}, "kebab-case")
}
