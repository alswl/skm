package services

// BuiltinProviderDefinitions returns the stable declarations used by skm.
func BuiltinProviderDefinitions() []BuiltinProviderDefinition {
	return []BuiltinProviderDefinition{
		localBuiltinDefinition,
		selfBuildBuiltinDefinition,
		githubBuiltinDefinition,
		gitlabBuiltinDefinition,
		skillsShBuiltinDefinition,
	}
}

// BuiltinProviders materializes providers in the documented matching order.
func BuiltinProviders() ([]Provider, error) {
	defs := BuiltinProviderDefinitions()
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
