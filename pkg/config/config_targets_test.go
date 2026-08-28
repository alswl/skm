package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestLoadReadsTargetsFromConfigYAML(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))
	cfgDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, SettingsFileName), []byte(`
root: ignored-by-explicit-root
targets:
  - name: acme
    platform: acme
    path: /custom/acme/skills
    accepts: [skill]
    strategies:
      skill: skill-symlink
`), 0o644))

	cfg, err := Load(root, cfgDir)
	require.NoError(t, err)
	byName := map[string]common.InstallTarget{}
	for _, target := range cfg.Targets {
		byName[target.Name] = target
	}
	require.Equal(t, "/custom/acme/skills", byName["acme"].Path)
	require.Contains(t, byName, "codex", "built-ins remain available")
}

func TestLoadDoesNotReadTargetsJSON(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))
	cfgDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "targets.json"), []byte(`[{"name":"old","path":"/old","kind":"skill"}]`), 0o644))

	cfg, err := Load(root, cfgDir)
	require.NoError(t, err)
	for _, target := range cfg.Targets {
		require.NotEqual(t, "old", target.Name)
	}
}
