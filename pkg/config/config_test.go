package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultConfigDir_XDGConfigHomeOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	require.Equal(t, filepath.Join(xdg, "skm"), DefaultConfigDir())
}

func TestDefaultConfigDir_FallsBackToDotConfigWhenXDGUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	require.Equal(t, filepath.Join(home, ".config", "skm"), DefaultConfigDir())
}

func TestDefaultPluginDirs_FollowsXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	require.Equal(t, []string{filepath.Join(xdg, "skm", "plugins")}, DefaultPluginDirs())
}
