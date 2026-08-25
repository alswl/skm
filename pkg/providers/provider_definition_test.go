package providers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuiltinDefinitionValidation(t *testing.T) {
	require.Error(t, (BuiltinDefinition{}).Validate())
	require.Error(t, (BuiltinDefinition{ID: "local"}).Validate())
	_, err := (BuiltinDefinition{ID: "local", New: func() Provider { return nil }}).Materialize()
	require.Error(t, err)
	require.NoError(t, (BuiltinDefinition{ID: "local", New: func() Provider { return NewLocal() }}).Validate())
}
