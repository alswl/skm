package config

import (
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestDefaultTargetsDshAndAgentsResolveEnvOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dshHome := t.TempDir()
	agentsHome := t.TempDir()
	t.Setenv("DSH_HOME", dshHome)
	t.Setenv("DSH_AGENTS_HOME", agentsHome)

	byName := map[string]common.InstallTarget{}
	for _, target := range DefaultTargets() {
		byName[target.Name] = target
	}
	require.Equal(t, filepath.Join(dshHome, "skills"), byName["dsh"].Path)
	require.Equal(t, filepath.Join(agentsHome, "skills"), byName["agents"].Path)
}

func TestDefaultTargetsDshAndAgentsFallBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DSH_HOME", "")
	t.Setenv("DSH_AGENTS_HOME", "")

	byName := map[string]common.InstallTarget{}
	for _, target := range DefaultTargets() {
		byName[target.Name] = target
	}
	require.Equal(t, filepath.Join(home, ".dsh", "skills"), byName["dsh"].Path)
	require.Equal(t, filepath.Join(home, ".agents", "skills"), byName["agents"].Path)
}

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
