package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func mkdir(t *testing.T, p string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(p, 0o755))
}

func write(t *testing.T, p, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

// newTestInstaller builds a temp repo entry + targets and an Installer.
func newTestInstaller(t *testing.T, entryKind common.EntryKind) (*common.Entry, common.InstallTarget, *Installer) {
	t.Helper()
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "local", "demo")
	mkdir(t, skillDir)
	marker := "SKILL.md"
	if entryKind == common.KindCommand {
		marker = "command.md"
	}
	write(t, filepath.Join(skillDir, marker), "---\nname: demo\ndescription: a demo\n---\nbody\n")
	write(t, filepath.Join(skillDir, "assets", "x.txt"), "resource")
	entry := &common.Entry{Name: "demo", Kind: entryKind, Path: skillDir}
	if entryKind == common.KindCommand {
		entry.Path = skillDir
	}
	target := common.InstallTarget{Name: "t", Path: filepath.Join(root, "targets", "t"), Kind: entryKind}
	mkdir(t, target.Path)
	return entry, target, NewInstaller([]common.InstallTarget{target}, nil)
}

func TestInstallSkillCreatesDirSymlinkAndIsIdempotent(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindSkill)
	tx := &dal.FileTransaction{}
	changed, err := inst.Install(tx, entry, target, false)
	require.NoError(t, err)
	require.True(t, changed)
	tx.Commit()

	link := filepath.Join(target.Path, "demo")
	require.True(t, dal.IsSymlink(link))
	require.Equal(t, entry.Path, dal.ResolveLink(link))
	require.Equal(t, common.InstallInstalled, inst.State(entry, target))

	// Idempotent re-install is a no-op.
	tx2 := &dal.FileTransaction{}
	changed, err = inst.Install(tx2, entry, target, false)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestInstallCodexCommandCreatesDirSymlink(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindCommand)
	target.Name = "codex"
	target.Kind = ""
	target.Accepts = []common.EntryKind{common.KindCommand}
	target.Strategies = map[common.EntryKind]common.InstallStrategy{
		common.KindCommand: common.StrategyCommandSymlink,
	}

	tx := &dal.FileTransaction{}
	changed, err := inst.Install(tx, entry, target, false)
	require.NoError(t, err)
	require.True(t, changed)
	tx.Commit()

	link := filepath.Join(target.Path, entry.Name)
	require.True(t, dal.IsSymlink(link))
	require.Equal(t, entry.Path, dal.ResolveLink(link))
	require.Equal(t, common.InstallInstalled, inst.State(entry, target))
}

func TestInstallClaudeCommandCreatesMarkdownSymlink(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindCommand)
	target.Kind = common.KindCommand // command target
	tx := &dal.FileTransaction{}
	changed, err := inst.Install(tx, entry, target, false)
	require.NoError(t, err)
	require.True(t, changed)
	tx.Commit()

	link := filepath.Join(target.Path, "demo.md")
	require.True(t, dal.IsSymlink(link))
	require.Equal(t, entry.MarkerPath(), dal.ResolveLink(link))
	require.Equal(t, common.InstallInstalled, inst.State(entry, target))
}

func TestInstallCommandAdapter(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindCommand)
	target.Kind = common.KindSkill // e.g. a Codex skill target
	tx := &dal.FileTransaction{}
	changed, err := inst.Install(tx, entry, target, false)
	require.NoError(t, err)
	require.True(t, changed)
	tx.Commit()

	adapter := filepath.Join(target.Path, "demo")
	require.True(t, dal.IsDir(adapter))
	require.FileExists(t, filepath.Join(adapter, dal.AdapterMarker), "adapter must carry the marker")
	require.False(t, dal.IsSymlink(filepath.Join(adapter, "SKILL.md")))
	actual, err := os.ReadFile(filepath.Join(adapter, "SKILL.md"))
	require.NoError(t, err)
	expected, err := os.ReadFile(entry.MarkerPath())
	require.NoError(t, err)
	require.Equal(t, string(expected), string(actual))
	require.Equal(t, filepath.Join(entry.Path, "assets"), dal.ResolveLink(filepath.Join(adapter, "assets")),
		"auxiliary resource tree must be linked through the adapter")
	require.True(t, dal.PathExists(filepath.Join(adapter, "assets", "x.txt")),
		"auxiliary resource must stay visible through the adapter")
	require.Equal(t, common.InstallInstalled, inst.State(entry, target))
}

func TestInstallCommandAdapterReplacesLegacyDirectorySymlink(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindCommand)
	target.Kind = common.KindSkill // legacy Codex command target shape
	legacy := filepath.Join(target.Path, entry.Name)
	require.NoError(t, os.Symlink(entry.Path, legacy))

	tx := &dal.FileTransaction{}
	_, err := inst.Install(tx, entry, target, false)
	require.Error(t, err, "the legacy directory symlink must require force to replace")
	_ = tx.Rollback()

	tx = &dal.FileTransaction{}
	changed, err := inst.Install(tx, entry, target, true)
	require.NoError(t, err)
	require.True(t, changed)
	tx.Commit()

	require.True(t, dal.IsDir(legacy))
	require.False(t, dal.IsSymlink(legacy))
	require.False(t, dal.IsSymlink(filepath.Join(legacy, "SKILL.md")))
	require.Equal(t, common.InstallInstalled, inst.State(entry, target))
}

// TestInstallCommandAdapterForSingleFileCommand: a single-file command's
// entry.Path is the .md marker file itself, not a directory (Entry.MarkerPath
// doc). entryResources used to unconditionally os.ReadDir(entry.Path), which
// fails with ENOTDIR for a plain file — installAdapter crashed on every
// single-file command with "list resources for %q: ...: not a dir". A
// single-file command has no sibling resources to carry through the adapter.
func TestInstallCommandAdapterForSingleFileCommand(t *testing.T) {
	root := t.TempDir()
	markerPath := filepath.Join(root, "commands", "local", "flatcmd.md")
	write(t, markerPath, "---\nname: flatcmd\ndescription: a single-file command\n---\nbody\n")
	entry := &common.Entry{Name: "flatcmd", Kind: common.KindCommand, Path: markerPath}
	target := common.InstallTarget{Name: "t", Path: filepath.Join(root, "targets", "t"), Kind: common.KindSkill}
	mkdir(t, target.Path)
	inst := NewInstaller([]common.InstallTarget{target}, nil)

	tx := &dal.FileTransaction{}
	changed, err := inst.Install(tx, entry, target, false)
	require.NoError(t, err, "installing a single-file command via the adapter strategy must not crash")
	require.True(t, changed)
	tx.Commit()

	adapter := filepath.Join(target.Path, "flatcmd")
	require.True(t, dal.IsDir(adapter))
	require.FileExists(t, filepath.Join(adapter, dal.AdapterMarker))
	require.False(t, dal.IsSymlink(filepath.Join(adapter, "SKILL.md")))
	actual, err := os.ReadFile(filepath.Join(adapter, "SKILL.md"))
	require.NoError(t, err)
	expected, err := os.ReadFile(entry.MarkerPath())
	require.NoError(t, err)
	require.Equal(t, string(expected), string(actual))
	require.Equal(t, common.InstallInstalled, inst.State(entry, target))
}

func TestInstallRefusesConflictWithoutForce(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindSkill)
	conflict := filepath.Join(target.Path, "demo")
	write(t, conflict, "user real file")

	tx := &dal.FileTransaction{}
	_, err := inst.Install(tx, entry, target, false)
	require.Error(t, err, "install over a user file must be refused without --force")
	content, _ := os.ReadFile(conflict)
	require.Equal(t, "user real file", string(content), "user file must be untouched on refusal")
}

func TestInstallForceOverwritesConflict(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindSkill)
	conflict := filepath.Join(target.Path, "demo")
	write(t, conflict, "user real file")

	tx := &dal.FileTransaction{}
	changed, err := inst.Install(tx, entry, target, true)
	require.NoError(t, err)
	require.True(t, changed)
	tx.Commit()
	require.Equal(t, common.InstallInstalled, inst.State(entry, target))
}

func TestUninstallManagedOnlyNeverDeletesUserFile(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindSkill)
	// A real user file at the link path must survive uninstall.
	userFile := filepath.Join(target.Path, "demo")
	write(t, userFile, "user data")

	tx := &dal.FileTransaction{}
	changed, err := inst.Uninstall(tx, entry, target)
	require.NoError(t, err)
	require.False(t, changed, "non-managed object must not be uninstalled")
	content, _ := os.ReadFile(userFile)
	require.Equal(t, "user data", string(content))

	// Install (force) then uninstall removes only the managed link.
	tx2 := &dal.FileTransaction{}
	_, err = inst.Install(tx2, entry, target, true)
	require.NoError(t, err)
	tx2.Commit()
	require.Equal(t, common.InstallInstalled, inst.State(entry, target))

	tx3 := &dal.FileTransaction{}
	changed, err = inst.Uninstall(tx3, entry, target)
	require.NoError(t, err)
	require.True(t, changed)
	tx3.Commit()
	require.False(t, dal.PathExists(userFile), "managed link must be removed")
}

func TestDanglingLinkReportedNotInstalled(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindSkill)
	// A symlink to a foreign source is dangling, not healthy.
	foreign := filepath.Join(t.TempDir(), "foreign")
	mkdir(t, foreign)
	link := filepath.Join(target.Path, "demo")
	require.NoError(t, os.Symlink(foreign, link))

	require.Equal(t, common.InstallDangling, inst.State(entry, target))
}

func TestBrokenLinkReportedDangling(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindSkill)
	link := filepath.Join(target.Path, "demo")
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "gone"), link))
	require.Equal(t, common.InstallDangling, inst.State(entry, target))
}

func TestUninstallRemovesLegacySelfBuildDanglingLink(t *testing.T) {
	root := t.TempDir()
	providerID := "self-build"
	entry := &common.Entry{
		Name:       "demo",
		Kind:       common.KindSkill,
		Path:       filepath.Join(root, "skills", "self-build", "demo"),
		ProviderID: &providerID,
	}
	target := common.InstallTarget{Name: "t", Path: filepath.Join(root, "target"), Kind: common.KindSkill}
	mkdir(t, target.Path)
	link := filepath.Join(target.Path, entry.Name)
	legacyPath := filepath.Join(root, "skills", entry.Name)
	require.NoError(t, os.Symlink(legacyPath, link))

	tx := &dal.FileTransaction{}
	changed, err := NewInstaller([]common.InstallTarget{target}, nil).Uninstall(tx, entry, target)
	require.NoError(t, err)
	require.True(t, changed)
	tx.Commit()
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err), "the exact old self-build link is safe to remove")
}

func TestUninstallLeavesUnrecognizedDanglingLink(t *testing.T) {
	entry, target, inst := newTestInstaller(t, common.KindSkill)
	link := filepath.Join(target.Path, entry.Name)
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "gone"), link))

	tx := &dal.FileTransaction{}
	changed, err := inst.Uninstall(tx, entry, target)
	require.NoError(t, err)
	require.False(t, changed)
	_, err = os.Lstat(link)
	require.NoError(t, err, "an arbitrary dangling user link must remain untouched")
}
