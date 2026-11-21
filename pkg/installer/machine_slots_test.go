package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestMachineTargetsAndSlotObservations(t *testing.T) {
	root := t.TempDir()
	targets := MachineDirectoryTargets(root)
	require.Equal(t, []MachineTarget{{Name: "codex", Path: filepath.Join(root, ".agents", "skills")}, {Name: "claude", Path: filepath.Join(root, ".claude", "skills")}}, targets)
	configured := []common.InstallTarget{
		{Name: "direct", Path: root, Accepts: []common.EntryKind{common.KindSkill}, Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink}},
		{Name: "plugin", Path: root, Accepts: []common.EntryKind{common.KindSkill}, Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: "plugin:other"}},
	}
	require.Equal(t, []MachineTarget{{Name: "direct", Path: root}}, MachineUserTargets(configured))
	source := filepath.Join(root, "source")
	require.NoError(t, os.Mkdir(source, 0o755))
	index, err := MachineSourceIndex([]MachineSource{{Path: source, Skill: "skills/local/source"}})
	require.NoError(t, err)
	alias := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(source, alias))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(alias, link))
	got := InspectMachineSlot(link, index)
	require.Equal(t, "installed", got.Status)
	require.Equal(t, "skills/local/source", got.Skill)
	require.Equal(t, "missing", InspectMachineSlot(filepath.Join(root, "missing"), index).Status)
	require.NoError(t, os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "dangling")))
	require.Equal(t, "dangling", InspectMachineSlot(filepath.Join(root, "dangling"), index).Status)
	require.NoError(t, os.WriteFile(filepath.Join(root, "external"), []byte("x"), 0o600))
	require.Equal(t, "external", InspectMachineSlot(filepath.Join(root, "external"), index).Status)
	require.Equal(t, "error", InspectMachineSlot(filepath.Join(root, "external", "child"), index).Status)
}

func TestMachineSourceIndexRejectsAmbiguousAlias(t *testing.T) {
	root := t.TempDir()
	good := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(root, alias))
	index, diagnostics := InspectMachineSources([]MachineSource{{Path: root, Skill: "one"}, {Path: alias, Skill: "two"}, {Path: filepath.Join(root, "missing"), Skill: "missing"}, {Path: good, Skill: "good"}})
	physical, err := filepath.EvalSymlinks(good)
	require.NoError(t, err)
	require.Equal(t, map[string]string{physical: "good"}, index)
	require.Len(t, diagnostics, 2)
}
