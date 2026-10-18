package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeployCommandArgumentFailuresAreJSON(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--repo", "./local"}, {"--repo", "./local", "--target", "t", "--into", "x", "--no-repo"},
		{"--repo", "./local", "--target", "t", "--root", "x"}, {"--repo", "./local", "--target", "t", "--into", ""},
		{"--repo", "./local", "--target", "t", "--root", "x", "--into", "y"},
		{"--repo", "./local", "--target", "t", "--root", "x", "--no-repo"},
		{"unexpected"},
	} {
		t.Run("arguments", func(t *testing.T) {
			argv := append([]string{"deploy", "--json"}, args...)
			out, err := runCmd(t, argv...)
			require.Error(t, err)
			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(out), &result))
			require.Equal(t, false, result["success"])
			require.Equal(t, "failed", result["phase"])
		})
	}
}

func TestDeployCommandDefaultUsesInitializedRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SKM_PLUGINS_DIR", t.TempDir())
	t.Setenv("SKM_ROOT", "")
	src, cfg := cmdFixture(t)
	_, err := runCmd(t, "deploy", "--repo", src, "--target", "t", "--config", cfg, "--json")
	require.ErrorContains(t, err, "skm init")
	_, err = runCmd(t, "init")
	require.NoError(t, err)
	out, err := runCmd(t, "deploy", "--repo", src, "--target", "t", "--config", cfg, "--json")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	cwd, err := os.Getwd()
	require.NoError(t, err)
	expected, err := filepath.EvalSymlinks(cwd)
	require.NoError(t, err)
	require.Equal(t, expected, result["destination"])
	require.Equal(t, []any{"t"}, result["targets"])
	require.NoDirExists(t, filepath.Join(cwd, "skm"))
	out, err = runCmd(t, "deploy", "--repo", src, "--target", "t", "--config", cfg, "--json")
	require.Error(t, err)
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, false, result["success"])
}

func TestDeployCommandAcceptsPlatformAndDeduplicatesTargets(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SKM_PLUGINS_DIR", t.TempDir())
	t.Setenv("SKM_ROOT", "")
	_, err := runCmd(t, "init")
	require.NoError(t, err)
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: demo\ndescription: test\n---\nbody"), 0o644))
	out, err := runCmd(t, "deploy", "--repo", src, "--target", "codex,claude-skills,claude", "--json")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, []any{"codex", "claude-skills"}, result["targets"])
	require.Equal(t, true, result["success"])
	require.FileExists(t, filepath.Join(home, ".claude/skills/demo/SKILL.md"))
}

func TestDeployCommandExplicitRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("SKM_ROOT", "")
	src, cfg := cmdFixture(t)
	library := filepath.Join(t.TempDir(), "library")
	_, err := runCmd(t, "init", library)
	require.NoError(t, err)
	out, err := runCmd(t, "deploy", "--repo", src, "--root", library, "--target", "t", "--config", cfg, "--json")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	expected, err := filepath.EvalSymlinks(library)
	require.NoError(t, err)
	require.Equal(t, expected, result["destination"])
	require.FileExists(t, filepath.Join(library, "skills/local/skill-a/SKILL.md"))
}
