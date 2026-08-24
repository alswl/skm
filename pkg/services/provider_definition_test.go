package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuiltinProviderDefinitionValidation(t *testing.T) {
	require.Error(t, (BuiltinProviderDefinition{}).Validate())
	require.Error(t, (BuiltinProviderDefinition{ID: "local"}).Validate())
	_, err := (BuiltinProviderDefinition{ID: "local", New: func() Provider { return nil }}).Materialize()
	require.Error(t, err)
	require.NoError(t, (BuiltinProviderDefinition{ID: "local", New: func() Provider { return NewLocal() }}).Validate())
}
