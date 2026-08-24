package services

import "fmt"

// BuiltinProviderDefinition declares one in-process provider without adding a
// second provider interface or a persisted configuration type.
type BuiltinProviderDefinition struct {
	ID  string
	New func() Provider
}

func (d BuiltinProviderDefinition) Validate() error {
	if d.ID == "" {
		return fmt.Errorf("provider definition: id must be non-empty")
	}
	if d.New == nil {
		return fmt.Errorf("provider definition %q: constructor must be non-nil", d.ID)
	}
	return nil
}

func (d BuiltinProviderDefinition) Materialize() (Provider, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	p := d.New()
	if p == nil {
		return nil, fmt.Errorf("provider definition %q: constructor returned nil", d.ID)
	}
	if p.ID() != d.ID {
		return nil, fmt.Errorf("provider definition %q: constructor returned %q", d.ID, p.ID())
	}
	return p, nil
}
