package commands

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

// T021: target list/add/update/remove/validate JSON goldens
// (contracts/cli-json.md). $HOME is isolated (a fresh temp dir) so the
// built-in default target paths are deterministic in shape but not in
// literal value; the random home is normalized to the $HOME sentinel before
// comparing against the committed golden, mirroring normalizeRoot in
// read_test.go.

func TestTargetListDefaultsJSONGolden(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := t.TempDir()
	out, err := runCmd(t, "target", "list", "--config", cfgDir, "--json")
	require.NoError(t, err)
	normalized := strings.ReplaceAll(out, home, "$HOME")
	normalized = strings.ReplaceAll(normalized, cfgDir, "$CFG")
	assertGoldenJSON(t, []byte(normalized), "../../testdata/golden/target-list-defaults.json")
}

func TestTargetAddUpdateRemoveLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfgDir := t.TempDir()

	out, err := runCmd(t, "target", "add",
		"--name", "my-tool", "--platform", "mytool", "--path", "/opt/mytool/skills",
		"--accepts", "skill,command",
		"--strategy", "skill=skill-symlink", "--strategy", "command=command-adapter",
		"--config", cfgDir, "--json")
	require.NoError(t, err)
	var addRep struct {
		Added   common.InstallTarget `json:"added"`
		Success bool                 `json:"success"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &addRep))
	require.True(t, addRep.Success)
	require.Equal(t, "my-tool", addRep.Added.Name)
	require.Equal(t, common.StrategyCommandAdapter, addRep.Added.Strategies[common.KindCommand])

	// Duplicate add is rejected.
	_, err = runCmd(t, "target", "add",
		"--name", "my-tool", "--platform", "mytool", "--path", "/opt/mytool/skills",
		"--accepts", "skill", "--strategy", "skill=skill-symlink",
		"--config", cfgDir, "--json")
	require.Error(t, err, "duplicate target name must be rejected")

	out, err = runCmd(t, "target", "update", "--name", "my-tool", "--path", "/opt/mytool/skills2",
		"--config", cfgDir, "--json")
	require.NoError(t, err)
	var updateRep struct {
		Updated common.InstallTarget `json:"updated"`
		Success bool                 `json:"success"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &updateRep))
	require.True(t, updateRep.Success)
	require.Equal(t, "/opt/mytool/skills2", updateRep.Updated.Path)

	out, err = runCmd(t, "target", "validate", "my-tool", "--config", cfgDir, "--json")
	require.NoError(t, err)
	var validateRep struct {
		Results []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"results"`
		Success bool `json:"success"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &validateRep))
	require.True(t, validateRep.Success)
	require.Len(t, validateRep.Results, 1)
	require.True(t, validateRep.Results[0].OK)

	out, err = runCmd(t, "target", "remove", "--name", "my-tool", "--config", cfgDir, "--json")
	require.NoError(t, err)
	var removeRep struct {
		Removed string `json:"removed"`
		Success bool   `json:"success"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &removeRep))
	require.True(t, removeRep.Success)
	require.Equal(t, "my-tool", removeRep.Removed)

	// Removing an already-gone target fails.
	_, err = runCmd(t, "target", "remove", "--name", "my-tool", "--config", cfgDir, "--json")
	require.Error(t, err)
}

func TestTargetValidateReportsUnknownNameAsFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfgDir := t.TempDir()
	_, err := runCmd(t, "target", "validate", "nonexistent", "--config", cfgDir, "--json")
	require.Error(t, err)
}
