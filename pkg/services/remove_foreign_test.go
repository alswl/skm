package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

// TestRemoveForeignDelegatesToPlugin: a plugin-backed target (strategy
// "plugin:<id>", e.g. acme) cleans a conflict through its own
// remove_foreign action, the same way the built-in strategies do — so the
// installs picker's "uninstall a conflict" works for external providers, not
// just claude-skills.
func TestRemoveForeignDelegatesToPlugin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.py")
	script := `#!/usr/bin/env python3
import json, shutil, sys
from pathlib import Path
req = json.loads(sys.stdin.readline())
action = req.get("action")
slot = Path(req.get("target_path", "")) / req.get("name", "")
if action == "id":
    print(json.dumps({"id": "fake", "protocol_version": 2}))
elif action == "label":
    print(json.dumps({"label": "fake"}))
elif action == "capability":
    print(json.dumps({"kinds": ["skill"]}))
elif action == "state":
    print(json.dumps({"state": "conflict" if slot.is_symlink() or slot.exists() else "absent"}))
elif action == "remove_foreign":
    if slot.is_dir():
        shutil.rmtree(slot)
    elif slot.is_symlink() or slot.is_file():
        slot.unlink()
    print(json.dumps({"result": True}))
else:
    print(json.dumps({"error": {"code": "protocol_error", "message": "unsupported"}}))
`
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	p, err := NewTargetPlugin(path)
	require.NoError(t, err)
	driver := externalTargetDriver{TargetPlugin: p}
	target := common.InstallTarget{
		Name: "fake", Path: filepath.Join(dir, "target"), Accepts: []common.EntryKind{common.KindSkill},
		Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.PluginStrategy("fake")},
	}
	inst := NewInstaller([]common.InstallTarget{target}, map[string]TargetDriver{"fake": driver})

	entry := &common.Entry{Name: "demo", Kind: common.KindSkill, Path: filepath.Join(dir, "src", "demo")}
	conflict := filepath.Join(target.Path, "demo")
	mkdir(t, conflict)
	write(t, filepath.Join(conflict, "occupied.txt"), "foreign")
	require.Equal(t, common.InstallConflict, inst.State(entry, target), "the plugin reports the foreign dir as a conflict")

	tx := &dal.FileTransaction{}
	changed, err := inst.RemoveForeign(tx, entry, target)
	require.NoError(t, err)
	require.True(t, changed, "the plugin removes the foreign object")
	tx.Commit()
	require.False(t, dal.PathExists(conflict), "the conflict is gone")
	require.Equal(t, common.InstallAbsent, inst.State(entry, target))
}

// TestRemoveForeignRejectsOldProtocolPlugin: a target plugin that declares
// protocol v1 (or predates the version field) gets a clear, actionable error
// when asked for remove_foreign — the v2-only action — instead of an opaque
// protocol failure, so the TUI can tell the user the plugin needs updating.
func TestRemoveForeignRejectsOldProtocolPlugin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.py")
	script := `#!/usr/bin/env python3
import json, sys
from pathlib import Path
req = json.loads(sys.stdin.readline())
action = req.get("action")
if action == "id":
    print(json.dumps({"id": "old"}))
elif action == "label":
    print(json.dumps({"label": "old"}))
else:
    print(json.dumps({"error": {"code": "protocol_error", "message": "unsupported"}}))
`
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	p, err := NewTargetPlugin(path)
	require.NoError(t, err)
	require.Equal(t, 1, p.ProtocolVersion(), "an undeclared protocol version is the v1 baseline")

	entry := &common.Entry{Name: "demo", Kind: common.KindSkill, Path: filepath.Join(dir, "src", "demo")}
	target := common.InstallTarget{Name: "old", Path: filepath.Join(dir, "target"), Accepts: []common.EntryKind{common.KindSkill}}

	_, err = p.RemoveForeign(entry, target)
	require.Error(t, err)
	require.Contains(t, err.Error(), "protocol v1", "the error names the plugin's version")
	require.Contains(t, err.Error(), "remove_foreign", "the error names the missing action")
	require.Contains(t, err.Error(), "v2", "the error names the required version")
}

// TestRemoveForeignRemovesConflictToAbsent: RemoveForeign backs up and removes
// the non-managed object occupying the entry's target path (the InstallConflict
// state), restoring the target to absent — the installs picker's "uninstall a
// conflict" transition.
func TestRemoveForeignRemovesConflictToAbsent(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindSkill)
	conflict := filepath.Join(target.Path, entry.Name)
	mkdir(t, conflict)
	write(t, filepath.Join(conflict, "user.txt"), "user data")
	require.Equal(t, common.InstallConflict, inst.State(entry, target))

	tx := &dal.FileTransaction{}
	changed, err := inst.RemoveForeign(tx, entry, target)
	require.NoError(t, err)
	require.True(t, changed)
	tx.Commit()
	require.False(t, dal.PathExists(conflict), "the foreign object is removed")
	require.Equal(t, common.InstallAbsent, inst.State(entry, target))
}

// TestRemoveForeignLeavesManagedInstallUntouched: a healthy managed install is
// not a conflict; RemoveForeign must never remove it.
func TestRemoveForeignLeavesManagedInstallUntouched(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindSkill)
	tx := &dal.FileTransaction{}
	_, err := inst.Install(tx, entry, target, false)
	require.NoError(t, err)
	tx.Commit()
	require.Equal(t, common.InstallInstalled, inst.State(entry, target))

	tx2 := &dal.FileTransaction{}
	changed, err := inst.RemoveForeign(tx2, entry, target)
	require.NoError(t, err)
	require.False(t, changed, "a managed install is not a foreign object")
	tx2.Commit()
	require.Equal(t, common.InstallInstalled, inst.State(entry, target), "managed install survives")
}

// TestRemoveForeignLeavesDanglingLinkUntouched: a dangling symlink is a broken
// managed-style link, which Uninstall removes, not RemoveForeign — the two
// transitions stay disjoint.
func TestRemoveForeignLeavesDanglingLinkUntouched(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindSkill)
	link := filepath.Join(target.Path, entry.Name)
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "gone"), link))
	require.Equal(t, common.InstallDangling, inst.State(entry, target))

	tx := &dal.FileTransaction{}
	changed, err := inst.RemoveForeign(tx, entry, target)
	require.NoError(t, err)
	require.False(t, changed, "a dangling link is Uninstall's job, not RemoveForeign's")
	tx.Commit()
	require.Equal(t, common.InstallDangling, inst.State(entry, target))
}
