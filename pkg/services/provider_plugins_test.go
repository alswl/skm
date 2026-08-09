package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

// writeStubPlugin writes an executable shell stub implementing the JSON
// subprocess protocol for the given id. capability/normalize are not
// implemented (fall through to "unknown action"), exercising the fallback
// path most plugins take.
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

// writeCapablePlugin writes a stub that additionally implements capability
// and normalize, and returns an {code,message} error object for fetch.
func writeCapablePlugin(t *testing.T, dir, name, id string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := `#!/bin/sh
IFS= read -r line
case "$line" in
  *'"action":"id"'*) echo '{"id":"` + id + `"}';;
  *'"action":"label"'*) echo '{"label":"Capable ` + id + `"}';;
  *'"action":"capability"'*) echo '{"description":"handles ` + id + `:// addresses","schemes":["` + id + `:"]}';;
  *'"action":"normalize"'*) echo '{"address":"normalized-by-` + id + `"}';;
  *'"action":"can_handle"'*) echo '{"result":true}';;
  *'"action":"fetch"'*) echo '{"error":{"code":"fetch_failed","message":"boom"}}';;
  *) echo '{"error":"unknown action"}';;
esac
`
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// writeLegacyErrorPlugin writes a stub whose fetch replies with a bare-string
// error (the pre-002 protocol shape), exercising the backward-compat mapping
// to CodeFetchFailed.
func writeLegacyErrorPlugin(t *testing.T, dir, name, id string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := `#!/bin/sh
IFS= read -r line
case "$line" in
  *'"action":"id"'*) echo '{"id":"` + id + `"}';;
  *'"action":"label"'*) echo '{"label":"Legacy ` + id + `"}';;
  *'"action":"can_handle"'*) echo '{"result":true}';;
  *'"action":"fetch"'*) echo '{"error":"boom"}';;
  *) echo '{"error":"unknown action"}';;
esac
`
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// writeSlowPlugin writes a stub whose `id` response never arrives (it sleeps
// far past any test timeout), exercising timeout isolation.
//
// The sleep is deliberately much longer than the timeout the test sets, so the
// healthy plugin discovered alongside it gets the whole budget to itself
// rather than racing the slow one's clock. `sleep`'s own stdout goes to
// /dev/null so it does not hold the pipe the shell hands back: when the
// deadline kills the shell, the pipe closes and exec.Cmd.Output returns at the
// deadline instead of waiting out the orphaned sleep. That is what keeps the
// test as quick as the timeout no matter how long the sleep is.
func writeSlowPlugin(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nsleep 300 >/dev/null 2>&1\necho '{\"id\":\"slow\"}'\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// writeBrokenJSONPlugin writes a stub that replies with invalid JSON,
// exercising the protocol_error isolation path.
func writeBrokenJSONPlugin(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\necho 'not json'\n"
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

// T009: capability/normalize parse correctly when a plugin implements them,
// and fall back to {id,label,"",nil} / identity when it doesn't.
func TestPluginCapabilityAndNormalize(t *testing.T) {
	dir := t.TempDir()

	capablePath := writeCapablePlugin(t, dir, "capable.sh", "acme")
	capable, err := NewPluginProvider(capablePath)
	require.NoError(t, err)
	cap := capable.Capability()
	require.Equal(t, "acme", cap.ID)
	require.Equal(t, "handles acme:// addresses", cap.Description)
	require.Equal(t, []string{"acme:"}, cap.Schemes)
	normalized, err := capable.Normalize("acme://org/repo")
	require.NoError(t, err)
	require.Equal(t, "normalized-by-acme", normalized)

	plainPath := writeStubPlugin(t, dir, "plain.sh", "plain")
	plain, err := NewPluginProvider(plainPath)
	require.NoError(t, err)
	fallback := plain.Capability()
	require.Equal(t, "plain", fallback.ID)
	require.Equal(t, "Stub plain", fallback.Label)
	require.Empty(t, fallback.Description)
	require.Empty(t, fallback.Schemes)
	same, err := plain.Normalize("unchanged-address")
	require.NoError(t, err)
	require.Equal(t, "unchanged-address", same, "normalize falls back to identity when unimplemented")
}

// T009 (error shape): fetch's typed {code,message} error is preserved, and a
// legacy bare-string error is mapped to CodeFetchFailed.
func TestPluginFetchErrorShapes(t *testing.T) {
	dir := t.TempDir()

	typedPath := writeCapablePlugin(t, dir, "typed.sh", "acme")
	typed, err := NewPluginProvider(typedPath)
	require.NoError(t, err)
	_, err = typed.Fetch(context.Background(), "addr")
	require.Error(t, err)
	var pe *ProviderError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, CodeFetchFailed, pe.Code)

	legacyPath := writeLegacyErrorPlugin(t, dir, "legacy.sh", "legacy")
	legacy, err := NewPluginProvider(legacyPath)
	require.NoError(t, err)
	_, err = legacy.Fetch(context.Background(), "addr")
	require.Error(t, err)
	require.ErrorAs(t, err, &pe)
	require.Equal(t, CodeFetchFailed, pe.Code, "bare-string legacy error maps to fetch_failed")
}

// T010: isolation — protocol errors and timeouts are recorded as typed
// ProviderLoadFailures and never prevent other providers from loading.
func TestDiscoverIsolatesProtocolErrorAndTimeout(t *testing.T) {
	dir := t.TempDir()
	providersDir := filepath.Join(dir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0o755))
	writeStubPlugin(t, providersDir, "ok.sh", "ok")
	writeBrokenJSONPlugin(t, providersDir, "broken-json.sh")
	slow := writeSlowPlugin(t, providersDir, "slow.sh")

	orig := pluginTimeout
	// Discovery loads every plugin concurrently (see
	// plugin_discovery_parallel_test.go), so ok.sh launches alongside slow.sh
	// and broken-json.sh instead of running alone as it did when discovery
	// loaded one plugin at a time. The timeout is therefore the budget ok.sh
	// must finish within while contending with two other subprocesses — and it
	// is far below slow.sh's sleep (writeSlowPlugin), so widening it costs
	// nothing but slack. At 1.5s against a 2s sleep, ok.sh had only 500ms of
	// headroom and was dropped as "timed out" on a loaded machine.
	// (The test's own duration is this timeout, since it waits for slow.sh to
	// hit it — so this trades a little wall clock for the headroom.)
	pluginTimeout = 3 * time.Second
	defer func() { pluginTimeout = orig }()

	plugins, failures := DiscoverPlugins([]string{dir}, common.NewLogger(false))
	require.Len(t, plugins, 1, "only the healthy plugin loads")
	require.Equal(t, "ok", plugins[0].ID())
	require.Len(t, failures, 2)

	byPath := map[string]ProviderLoadFailure{}
	for _, f := range failures {
		byPath[f.Path] = f
	}
	require.Equal(t, CodeProtocolError, byPath[filepath.Join(providersDir, "broken-json.sh")].Reason.Code)
	require.Equal(t, CodeTimeout, byPath[slow].Reason.Code)
}

func TestDiscoverRegistersPluginsInStableOrder(t *testing.T) {
	dir := t.TempDir()
	providersDir := filepath.Join(dir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0o755))
	writeStubPlugin(t, providersDir, "a.sh", "plugin-a")
	writeStubPlugin(t, providersDir, "b.sh", "plugin-b")
	logger := common.NewLogger(false)

	plugins, failures := DiscoverPlugins([]string{dir}, logger)
	require.Len(t, plugins, 2)
	require.Empty(t, failures)
	require.Equal(t, "plugin-a", plugins[0].ID())
	require.Equal(t, "plugin-b", plugins[1].ID())
}

func TestDiscoverIsolatesBrokenPlugin(t *testing.T) {
	dir := t.TempDir()
	providersDir := filepath.Join(dir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0o755))
	writeStubPlugin(t, providersDir, "ok.sh", "ok")
	broken := filepath.Join(providersDir, "broken.sh")
	require.NoError(t, os.WriteFile(broken, []byte("#!/bin/sh\nexit 1\n"), 0o755))

	logger := common.NewLogger(false)
	plugins, failures := DiscoverPlugins([]string{dir}, logger)
	require.Len(t, plugins, 1, "broken plugin must be isolated, not crash startup")
	require.Equal(t, "ok", plugins[0].ID())
	require.Len(t, failures, 1)
	require.Equal(t, broken, failures[0].Path)
}

func TestDiscoverRejectsDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	providersDir := filepath.Join(dir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0o755))
	writeStubPlugin(t, providersDir, "first.sh", "dup")
	writeStubPlugin(t, providersDir, "second.sh", "dup")

	logger := common.NewLogger(false)
	plugins, failures := DiscoverPlugins([]string{dir}, logger)
	require.Len(t, plugins, 1, "duplicate id rejected in favor of the first")
	require.Equal(t, "first.sh", filepath.Base(plugins[0].(*PluginProvider).path))
	require.Len(t, failures, 1)
	require.Equal(t, CodeDuplicateID, failures[0].Reason.Code)
}

func TestRegistryRegistrationOrder(t *testing.T) {
	dir := t.TempDir()
	providersDir := filepath.Join(dir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0o755))
	writeStubPlugin(t, providersDir, "p.sh", "plug")
	reg := NewRegistry()
	require.NoError(t, reg.Register(NewLocal()))
	require.NoError(t, reg.Register(NewGitHub()))
	loaded, _ := DiscoverPlugins([]string{dir}, common.NewLogger(false))
	for _, p := range loaded {
		require.NoError(t, reg.Register(p))
	}

	// Built-ins first, then the plugin.
	ids := []string{}
	for _, p := range reg.Providers() {
		ids = append(ids, p.ID())
	}
	require.Equal(t, []string{"local", "github", "plug"}, ids)
}
