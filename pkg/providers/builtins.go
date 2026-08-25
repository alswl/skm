package providers

// BuiltinDefinitions returns the five built-in provider declarations in their
// public order.
func BuiltinDefinitions() []BuiltinDefinition {
	return []BuiltinDefinition{
		localBuiltinDefinition,
		selfBuildBuiltinDefinition,
		githubBuiltinDefinition,
		gitlabBuiltinDefinition,
		skillsShBuiltinDefinition,
	}
}

// Builtins materializes every built-in provider so Services.New and tests
// share one matching order.
func Builtins() ([]Provider, error) {
	defs := BuiltinDefinitions()
	providers := make([]Provider, 0, len(defs))
	for _, definition := range defs {
		provider, err := definition.Materialize()
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, nil
}
