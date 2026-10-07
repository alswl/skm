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

func TestDeployCommandDefaultDestinationAndTargetSelection(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SKM_PLUGINS_DIR", t.TempDir())
	t.Setenv("SKM_ROOT", filepath.Join(t.TempDir(), "must-not-use"))
	src, cfg := cmdFixture(t)
	out, err := runCmd(t, "deploy", "--repo", src, "--target", "t", "--config", cfg, "--json")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	cwd, err := os.Getwd()
	require.NoError(t, err)
	expected, err := filepath.EvalSymlinks(filepath.Join(cwd, "skm"))
	require.NoError(t, err)
	require.Equal(t, expected, result["destination"])
	require.Equal(t, []any{"t"}, result["targets"])
	require.DirExists(t, filepath.Join(cwd, "skm"))
	out, err = runCmd(t, "deploy", "--repo", src, "--target", "t", "--config", cfg, "--force", "--json")
	require.Error(t, err)
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, false, result["success"])
}
