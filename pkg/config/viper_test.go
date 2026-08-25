package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettingsPrecedenceForRoot(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("root: /from/file\n"), 0o644))

	t.Run("file when nothing else is set", func(t *testing.T) {
		t.Setenv(EnvRoot, "")
		require.Equal(t, "/from/file", rootFrom(newSettings(dir), ""))
	})
	t.Run("env beats file", func(t *testing.T) {
		t.Setenv(EnvRoot, "/from/env")
		require.Equal(t, "/from/env", rootFrom(newSettings(dir), ""))
	})
	t.Run("flag beats env", func(t *testing.T) {
		t.Setenv(EnvRoot, "/from/env")
		require.Equal(t, "/from/flag", rootFrom(newSettings(dir), "/from/flag"))
	})
}

// A directory with no settings file must behave exactly as before one existed.
func TestSettingsAbsentKeepsDefaults(t *testing.T) {
	t.Setenv(EnvRoot, "")
	t.Setenv(EnvPluginsDir, "")
	v := newSettings(t.TempDir())
	require.Equal(t, "", rootFrom(v, ""), "no root: DiscoverRoot still searches upward")
	require.Equal(t, DefaultPluginDirs(), pluginDirsFrom(v))
}

func TestPluginDirsFromFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("plugin_dirs:\n  - /from/file\n"), 0o644))

	t.Run("default dir stays first, file entries append", func(t *testing.T) {
		t.Setenv(EnvPluginsDir, "")
		got := pluginDirsFrom(newSettings(dir))
		require.Equal(t, append(DefaultPluginDirs(), "/from/file"), got)
	})
	t.Run("env replaces the file list and splits on the path separator", func(t *testing.T) {
		t.Setenv(EnvPluginsDir, "/a"+string(os.PathListSeparator)+"/b")
		got := pluginDirsFrom(newSettings(dir))
		require.Equal(t, append(DefaultPluginDirs(), "/a", "/b"), got)
	})
}

func TestPluginDirsExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("plugin_dirs:\n  - ~/from-file\n"), 0o644))

	t.Setenv(EnvPluginsDir, "")
	require.Equal(t, append(DefaultPluginDirs(), filepath.Join(home, "from-file")),
		pluginDirsFrom(newSettings(dir)))

	t.Setenv(EnvPluginsDir, "~/from-env")
	require.Equal(t, append(DefaultPluginDirs(), filepath.Join(home, "from-env")),
		pluginDirsFrom(newSettings(dir)))
}
