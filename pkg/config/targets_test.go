package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

// T020: v2 targets.json load/validate; add rejects duplicate name; update
// re-validates; remove deletes; kind-incompatible strategy is rejected.

func TestParseTargetsV2Shape(t *testing.T) {
	data := []byte(`[{
		"name": "my-tool", "platform": "mytool", "path": "/opt/mytool/skills",
		"accepts": ["skill", "command"],
		"strategies": {"skill": "skill-symlink", "command": "command-adapter"}
	}]`)
	valid, invalid, err := ParseTargets(data)
	require.NoError(t, err)
	require.Empty(t, invalid)
	require.Len(t, valid, 1)
	require.Equal(t, "my-tool", valid[0].Name)
	require.Equal(t, common.StrategySkillSymlink, valid[0].Strategies[common.KindSkill])
	require.Equal(t, common.StrategyCommandAdapter, valid[0].Strategies[common.KindCommand])
}

func TestParseTargetsMigratesLegacyV1Shape(t *testing.T) {
	data := []byte(`[
		{"name": "t-skill", "path": "/p1", "builtin": false, "kind": "skill"},
		{"name": "t-command", "path": "/p2", "builtin": false, "kind": "command"}
	]`)
	valid, invalid, err := ParseTargets(data)
	require.NoError(t, err)
	require.Empty(t, invalid)
	require.Len(t, valid, 2)

	byName := map[string]common.InstallTarget{}
	for _, v := range valid {
		byName[v.Name] = v
	}
	skillT := byName["t-skill"]
	require.ElementsMatch(t, []common.EntryKind{common.KindSkill, common.KindCommand}, skillT.Accepts)
	require.Equal(t, common.StrategySkillSymlink, skillT.Strategies[common.KindSkill])
	require.Equal(t, common.StrategyCommandAdapter, skillT.Strategies[common.KindCommand])

	commandT := byName["t-command"]
	require.Equal(t, []common.EntryKind{common.KindCommand}, commandT.Accepts)
	require.Equal(t, common.StrategyCommandMarker, commandT.Strategies[common.KindCommand])
}

func TestParseTargetsReportsInvalidEntriesIndividuallyAndKeepsTheRest(t *testing.T) {
	data := []byte(`[
		{"name": "good", "path": "/p1", "kind": "skill"},
		{"path": "/p2", "kind": "skill"},
		{"name": "bad-kind", "path": "/p3", "kind": "widget"}
	]`)
	valid, invalid, err := ParseTargets(data)
	require.NoError(t, err)
	require.Len(t, valid, 1, "the one interpretable entry still loads")
	require.Equal(t, "good", valid[0].Name)
	require.Len(t, invalid, 2, "each uninterpretable entry is reported individually")
}

func TestParseTargetsRejectsKindIncompatibleStrategy(t *testing.T) {
	data := []byte(`[{
		"name": "bad", "platform": "x", "path": "/p",
		"accepts": ["skill"],
		"strategies": {"skill": "command-marker"}
	}]`)
	valid, invalid, err := ParseTargets(data)
	require.NoError(t, err)
	require.Empty(t, valid)
	require.Len(t, invalid, 1)
	require.Contains(t, invalid[0].Reason, "not compatible")
}

func TestValidateTarget(t *testing.T) {
	valid := common.InstallTarget{
		Name: "t", Platform: "p", Path: "/x",
		Accepts:    []common.EntryKind{common.KindSkill},
		Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink},
	}
	require.Empty(t, ValidateTarget(valid))

	require.NotEmpty(t, ValidateTarget(common.InstallTarget{}), "empty name/path rejected")
}

func TestAddTargetRejectsDuplicateName(t *testing.T) {
	dir := t.TempDir()
	t1 := common.InstallTarget{
		Name: "dup", Platform: "p", Path: "/x",
		Accepts:    []common.EntryKind{common.KindSkill},
		Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink},
	}
	_, err := AddTarget(dir, t1)
	require.NoError(t, err)
	_, err = AddTarget(dir, t1)
	require.Error(t, err, "duplicate name must be rejected")
}

func TestUpdateTargetRevalidates(t *testing.T) {
	dir := t.TempDir()
	orig := common.InstallTarget{
		Name: "t", Platform: "p", Path: "/x",
		Accepts:    []common.EntryKind{common.KindSkill},
		Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink},
	}
	_, err := AddTarget(dir, orig)
	require.NoError(t, err)

	updated, err := UpdateTarget(dir, "t", func(t *common.InstallTarget) { t.Path = "/y" })
	require.NoError(t, err)
	require.Equal(t, "/y", updated.Path)

	_, err = UpdateTarget(dir, "t", func(t *common.InstallTarget) { t.Strategies[common.KindSkill] = common.StrategyCommandMarker })
	require.Error(t, err, "an incompatible strategy must be rejected on update")

	_, err = UpdateTarget(dir, "missing", func(t *common.InstallTarget) {})
	require.Error(t, err, "updating an unknown target must fail")
}

func TestRemoveTargetDeletesByName(t *testing.T) {
	dir := t.TempDir()
	t1 := common.InstallTarget{
		Name: "gone", Platform: "p", Path: "/x",
		Accepts:    []common.EntryKind{common.KindSkill},
		Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink},
	}
	_, err := AddTarget(dir, t1)
	require.NoError(t, err)

	require.NoError(t, RemoveTarget(dir, "gone"))
	valid, _, err := ParseTargets(readFile(t, filepath.Join(dir, targetsFileName)))
	require.NoError(t, err)
	require.Empty(t, valid)

	require.Error(t, RemoveTarget(dir, "gone"), "removing an already-gone target must fail")
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
