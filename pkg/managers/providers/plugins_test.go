package providers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

// writeStubPlugin writes an executable shell stub implementing the JSON
// subprocess protocol for the given id.
func writeStubPlugin(t *testing.T, dir, name, id string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := `#!/bin/sh
IFS= read -r line
case "$line" in
  *'"action":"id"'*) echo '{"id":"` + id + `"}';;
  *'"action":"label"'*) echo '{"label":"Stub ` + id + `"}';;
  *'"action":"can_handle"'*) echo '{"result":true}';;
  *'"action":"fetch"'*) echo '{"path":"/tmp/stub-staged-` + id + `"}';;
  *) echo '{"error":"unknown action"}';;
esac
`
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func TestPluginProviderProtocol(t *testing.T) {
	dir := t.TempDir()
	path := writeStubPlugin(t, dir, "stub.sh", "stub")
	p, err := NewPluginProvider(path)
	require.NoError(t, err)
	require.Equal(t, "stub", p.ID())
	require.Equal(t, "Stub stub", p.Label())
	require.True(t, p.CanHandle("anything"))
	staged, err := p.Fetch(context.Background(), "addr")
	require.NoError(t, err)
	require.Equal(t, "/tmp/stub-staged-stub", staged)
}

func TestDiscoverRegistersPluginsInStableOrder(t *testing.T) {
	dir := t.TempDir()
	writeStubPlugin(t, dir, "a.sh", "plugin-a")
	writeStubPlugin(t, dir, "b.sh", "plugin-b")
	logger := common.NewLogger(false)

	plugins := DiscoverPlugins([]string{dir}, logger)
	require.Len(t, plugins, 2)
	require.Equal(t, "plugin-a", plugins[0].ID())
	require.Equal(t, "plugin-b", plugins[1].ID())
}

func TestDiscoverIsolatesBrokenPlugin(t *testing.T) {
	dir := t.TempDir()
	writeStubPlugin(t, dir, "ok.sh", "ok")
	broken := filepath.Join(dir, "broken.sh")
	require.NoError(t, os.WriteFile(broken, []byte("#!/bin/sh\nexit 1\n"), 0o755))

	logger := common.NewLogger(false)
	plugins := DiscoverPlugins([]string{dir}, logger)
	require.Len(t, plugins, 1, "broken plugin must be isolated, not crash startup")
	require.Equal(t, "ok", plugins[0].ID())
}

func TestDiscoverRejectsDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	writeStubPlugin(t, dir, "first.sh", "dup")
	writeStubPlugin(t, dir, "second.sh", "dup")

	logger := common.NewLogger(false)
	plugins := DiscoverPlugins([]string{dir}, logger)
	require.Len(t, plugins, 1, "duplicate id rejected in favor of the first")
	require.Equal(t, "first.sh", filepath.Base(plugins[0].(*PluginProvider).path))
}

func TestRegistryRegistrationOrder(t *testing.T) {
	dir := t.TempDir()
	writeStubPlugin(t, dir, "p.sh", "plug")
	reg := NewRegistry()
	require.NoError(t, reg.Register(NewLocal()))
	require.NoError(t, reg.Register(NewGitHub()))
	for _, p := range DiscoverPlugins([]string{dir}, common.NewLogger(false)) {
		require.NoError(t, reg.Register(p))
	}

	// Built-ins first, then the plugin.
	ids := []string{}
	for _, p := range reg.Providers() {
		ids = append(ids, p.ID())
	}
	require.Equal(t, []string{"local", "github", "plug"}, ids)
}
