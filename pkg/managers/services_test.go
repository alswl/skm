package managers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

// T032: the git-host and skills.sh Providers register after Local/GitHub in
// the documented resolution order and appear in ProviderList.
func TestNewRegistersBuiltinProvidersInDocumentedOrder(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))
	svc, err := New(newCfg(root, nil), common.NewLogger(false))
	require.NoError(t, err)

	var ids []string
	for _, p := range svc.Registry.Providers() {
		ids = append(ids, p.ID())
	}
	require.Equal(t, []string{"local", "self-build", "github", "gitlab", "skills-sh"}, ids)

	rep := svc.ProviderList()
	require.Len(t, rep.Providers, 5)
	for _, id := range ids {
		found := false
		for _, p := range rep.Providers {
			if p.ID == id {
				found = true
				require.Equal(t, "builtin", p.Kind)
			}
		}
		require.True(t, found, "%s must appear in provider list", id)
	}
}
