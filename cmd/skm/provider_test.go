package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// T011: provider list/validate JSON goldens for the built-in-only case
// (contracts/cli-json.md), plus dynamic-path coverage of a loaded plugin and
// a failed plugin (which can't be a committed golden — the failure's path is
// a tempdir).

func TestProviderListJSONGolden(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate from any real ~/.config/skm/plugins
	cfgDir := t.TempDir()
	out, err := runCmd(t, "provider", "list", "--config", cfgDir, "--json")
	require.NoError(t, err)
	assertGoldenJSON(t, []byte(out), "../../testdata/golden/provider-list.json")
}

func TestProviderValidateJSONGolden(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfgDir := t.TempDir()
	out, err := runCmd(t, "provider", "validate", "--config", cfgDir, "--json")
	require.NoError(t, err)
	assertGoldenJSON(t, []byte(out), "../../testdata/golden/provider-validate.json")
}

// writeCLIStubPlugin writes an executable implementing id/label/capability.
func writeCLIStubPlugin(t *testing.T, dir, name, id string) {
	t.Helper()
	script := `#!/bin/sh
IFS= read -r line
case "$line" in
  *'"action":"id"'*) echo '{"id":"` + id + `"}';;
  *'"action":"label"'*) echo '{"label":"Stub ` + id + `"}';;
  *'"action":"capability"'*) echo '{"description":"a test plugin"}';;
  *) echo '{"error":"unknown action"}';;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755))
}

func TestProviderListShowsLoadedPluginAndTypedFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pluginDir := t.TempDir()
	providersDir := filepath.Join(pluginDir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0o755))
	writeCLIStubPlugin(t, providersDir, "acme", "acme")
	require.NoError(t, os.WriteFile(filepath.Join(providersDir, "broken"), []byte("#!/bin/sh\nexit 1\n"), 0o755))
	t.Setenv("SKM_PLUGINS_DIR", pluginDir)

	cfgDir := t.TempDir()
	out, err := runCmd(t, "provider", "list", "--config", cfgDir, "--json")
	require.NoError(t, err)

	var rep struct {
		Providers []struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Loaded bool   `json:"loaded"`
			Error  *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"providers"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &rep))
	require.Len(t, rep.Providers, 7) // local, self-build, github, gitlab, skills-sh, acme, broken

	byID := map[string]int{}
	for i, p := range rep.Providers {
		byID[p.ID] = i
	}
	acme := rep.Providers[byID["acme"]]
	require.Equal(t, "plugin", acme.Kind)
	require.True(t, acme.Loaded)
	require.Nil(t, acme.Error)

	// The broken plugin never returns an id, so it's keyed by "" among failures.
	var broken *struct {
		ID     string `json:"id"`
		Kind   string `json:"kind"`
		Loaded bool   `json:"loaded"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	for i := range rep.Providers {
		if !rep.Providers[i].Loaded {
			broken = &rep.Providers[i]
		}
	}
	require.NotNil(t, broken, "broken plugin must appear as a load failure, not silently vanish")
	require.False(t, broken.Loaded)
	require.NotNil(t, broken.Error)
	require.NotEmpty(t, broken.Error.Code)
	require.NotEmpty(t, broken.Error.Message)
}

func TestProviderValidateFailsWhenAPluginIsBroken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pluginDir := t.TempDir()
	providersDir := filepath.Join(pluginDir, "providers")
	require.NoError(t, os.MkdirAll(providersDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(providersDir, "broken"), []byte("#!/bin/sh\nexit 1\n"), 0o755))
	t.Setenv("SKM_PLUGINS_DIR", pluginDir)

	cfgDir := t.TempDir()
	out, err := runCmd(t, "provider", "validate", "--config", cfgDir, "--json")
	require.Error(t, err, "validate reports a non-zero exit when any provider fails")

	var rep struct {
		Success bool `json:"success"`
		Results []struct {
			OK bool `json:"ok"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &rep))
	require.False(t, rep.Success)
	found := false
	for _, r := range rep.Results {
		if !r.OK {
			found = true
		}
	}
	require.True(t, found)
}
