package providers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuiltinProviderRegistryOrder(t *testing.T) {
	providers, err := BuiltinProviders()
	require.NoError(t, err)
	ids := make([]string, 0, len(providers))
	for _, provider := range providers {
		ids = append(ids, provider.ID())
	}
	require.Equal(t, []string{"local", "self-build", "github", "gitlab", "skills-sh"}, ids)
}
