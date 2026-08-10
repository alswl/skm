package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func TestArchiveUnarchivePreservesLayout(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/team/demo/SKILL.md", frontmatter("demo", "a skill"))
	repo := NewRepository(root)
	entry := findEntryByName(t, root, "demo")

	newPath, err := repo.Archive(context.Background(), entry, LifecycleOptions{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "archived/local/team/demo"), newPath)
	require.FileExists(t, filepath.Join(root, "archived/local/team/demo/SKILL.md"))
	require.NoFileExists(t, filepath.Join(root, "skills/local/team/demo"))

	archived := findEntryByName(t, root, "demo")
	require.Equal(t, common.StatusArchived, archived.Status)

	restored, err := repo.Unarchive(context.Background(), archived, LifecycleOptions{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/local/team/demo"), restored)
	require.FileExists(t, filepath.Join(root, "skills/local/team/demo/SKILL.md"))
}

func TestArchiveDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "a skill"))
	entry := findEntryByName(t, root, "demo")

	_, err := NewRepository(root).Archive(context.Background(), entry, LifecycleOptions{DryRun: true})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "skills/local/demo/SKILL.md"))
	require.NoDirExists(t, filepath.Join(root, "archived"))
}

func TestDeleteRequiresForce(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "a skill"))
	entry := findEntryByName(t, root, "demo")

	err := NewRepository(root).Delete(context.Background(), entry, LifecycleOptions{})
	require.Error(t, err, "delete without --force must be refused")
	require.FileExists(t, filepath.Join(root, "skills/local/demo/SKILL.md"))

	require.NoError(t, NewRepository(root).Delete(context.Background(), entry, LifecycleOptions{Force: true}))
	require.NoDirExists(t, filepath.Join(root, "skills/local/demo"))
}

func TestConvertFlipsKindAndDropsOrigin(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "a skill"))
	writeFile(t, root, "skills/local/demo/meta.json", `{"address":"https://x","mode_id":"local"}`)
	entry := findEntryByName(t, root, "demo")

	newEntry, err := NewRepository(root).ConvertContent(context.Background(), entry, common.KindCommand)
	require.NoError(t, err)
	require.Equal(t, common.KindCommand, newEntry.Kind)
	require.Equal(t, filepath.Join(root, "commands/local/demo"), newEntry.Path)
	require.FileExists(t, filepath.Join(root, "commands/local/demo/command.md"), "marker rewritten")
	require.NoFileExists(t, filepath.Join(root, "commands/local/demo/SKILL.md"))
	require.NoFileExists(t, filepath.Join(root, "commands/local/demo/meta.json"), "origin removed")
	require.NoDirExists(t, filepath.Join(root, "skills/local/demo"))
	require.Nil(t, newEntry.Origin)
}

func TestConvertRejectsNonConvertible(t *testing.T) {
	// Single-file command cannot be converted.
	root := t.TempDir()
	writeFile(t, root, "commands/local/single.md", "---\nname: single\ndescription: s\n---\nbody\n")
	single := findEntryByName(t, root, "single")
	_, err := NewRepository(root).ConvertContent(context.Background(), single, common.KindSkill)
	require.Error(t, err)
	require.Contains(t, err.Error(), "single-file")

	// Archived entry cannot be converted.
	writeFile(t, root, "archived/local/old/SKILL.md", frontmatter("old", "archived"))
	old := findEntryByName(t, root, "old")
	_, err = NewRepository(root).ConvertContent(context.Background(), old, common.KindCommand)
	require.Error(t, err)
	require.FileExists(t, filepath.Join(root, "archived/local/old/SKILL.md"), "no half-state")
}

// TestNormalizeMovesSkillMissingProviderLevel: skills/<name>/SKILL.md
// (missing its provider directory) moves to skills/local/<name>/SKILL.md.
func TestNormalizeMovesSkillMissingProviderLevel(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/flat-skill/SKILL.md", frontmatter("flat-skill", "missing provider level"))
	entry := findEntryByName(t, root, "flat-skill")
	require.Equal(t, common.StatusNonStandard, entry.Status)

	dest, err := NewRepository(root).Normalize(context.Background(), entry, "", LifecycleOptions{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/local/flat-skill"), dest)
	require.FileExists(t, filepath.Join(root, "skills/local/flat-skill/SKILL.md"))
	require.NoFileExists(t, filepath.Join(root, "skills/flat-skill"))

	moved := findEntryByName(t, root, "flat-skill")
	require.Equal(t, common.StatusActive, moved.Status, "the entry is now in its standard location")
	require.Equal(t, "local", moved.ProviderIDValue())
}

// TestNormalizeMovesLooseCommandFileMissingProviderLevel:
// commands/<name>.md moves to commands/local/<name>.md, preserving its
// single-file shape.
func TestNormalizeMovesLooseCommandFileMissingProviderLevel(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "commands/loose.md", frontmatter("loose", "missing provider level"))
	entry := findEntryByName(t, root, "loose")
	require.Equal(t, common.StatusNonStandard, entry.Status)
	require.False(t, entry.IsDirectory())

	dest, err := NewRepository(root).Normalize(context.Background(), entry, "", LifecycleOptions{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "commands/local/loose.md"), dest)
	require.FileExists(t, dest)
	require.NoFileExists(t, filepath.Join(root, "commands/loose.md"))
}

// TestNormalizeDryRunWritesNothing previews the destination without moving.
func TestNormalizeDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/flat-skill/SKILL.md", frontmatter("flat-skill", "x"))
	entry := findEntryByName(t, root, "flat-skill")

	dest, err := NewRepository(root).Normalize(context.Background(), entry, "", LifecycleOptions{DryRun: true})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/local/flat-skill"), dest)
	require.FileExists(t, filepath.Join(root, "skills/flat-skill/SKILL.md"), "dry-run writes nothing")
	require.NoDirExists(t, dest)
}

// TestNormalizeRefusesDestinationConflict: refuses rather than overwriting
// when something already occupies the default "local" destination.
func TestNormalizeRefusesDestinationConflict(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/flat-skill/SKILL.md", frontmatter("flat-skill", "the misplaced one"))
	writeFile(t, root, "skills/local/flat-skill/SKILL.md", frontmatter("flat-skill", "already there"))
	// Two entries now share the name (a pre-existing name conflict, reported
	// separately by verify); find the non-standard one specifically.
	var nonStandard *common.Entry
	for _, e := range NewRepository(root).Scan() {
		if e.Status == common.StatusNonStandard {
			nonStandard = e
		}
	}
	require.NotNil(t, nonStandard)

	_, err := NewRepository(root).Normalize(context.Background(), nonStandard, "", LifecycleOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
	require.FileExists(t, filepath.Join(root, "skills/flat-skill/SKILL.md"), "no half-state: source untouched on refusal")
}

// TestNormalizeRejectsNonNonStandardEntry: only StatusNonStandard entries
// can be normalized.
func TestNormalizeRejectsNonNonStandardEntry(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/proper/SKILL.md", frontmatter("proper", "already standard"))
	entry := findEntryByName(t, root, "proper")
	require.Equal(t, common.StatusActive, entry.Status)

	_, err := NewRepository(root).Normalize(context.Background(), entry, "", LifecycleOptions{})
	require.Error(t, err)
}

// TestNormalizeMovesToExplicitProvider: choosing a provider other than the
// "local" default relocates the entry there instead.
func TestNormalizeMovesToExplicitProvider(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/flat-skill/SKILL.md", frontmatter("flat-skill", "belongs to github, really"))
	entry := findEntryByName(t, root, "flat-skill")

	dest, err := NewRepository(root).Normalize(context.Background(), entry, "github", LifecycleOptions{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/github/flat-skill"), dest)
	require.FileExists(t, filepath.Join(root, "skills/github/flat-skill/SKILL.md"))

	moved := findEntryByName(t, root, "flat-skill")
	require.Equal(t, "github", moved.ProviderIDValue())
}

// TestNormalizeActiveEntryRelinksInstalls: moving an active entry to another
// provider relinks its installs to the new location — the old link is removed
// and a new one points at the moved path (uninstall -> move -> reinstall),
// instead of leaving a dangling link at the old path.
func TestNormalizeActiveEntryRelinksInstalls(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/github/demo/SKILL.md", frontmatter("demo", "a demo skill"))
	targetDir := t.TempDir()
	cfg := newCfg(root, []common.InstallTarget{{Name: "t", Path: targetDir, Kind: common.KindSkill}})
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	entry := findEntryByName(t, root, "demo")
	require.Equal(t, common.StatusActive, entry.Status)
	require.Equal(t, "github", entry.ProviderIDValue())

	target, ok := svc.Installer.TargetByName("t")
	require.True(t, ok)
	tx := &dal.FileTransaction{}
	_, err = svc.Installer.Install(tx, entry, target, false)
	require.NoError(t, err)
	tx.Commit()
	link := filepath.Join(targetDir, "demo")
	require.Equal(t, common.InstallInstalled, svc.Installer.State(entry, target))
	resolved, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/github/demo"), resolved)

	res, err := svc.Normalize(context.Background(), "demo", "local", LifecycleOptions{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/local/demo"), res.Path)

	// The link now points at the moved location; the old tree is gone.
	moved := findEntryByName(t, root, "demo")
	require.Equal(t, common.InstallInstalled, svc.Installer.State(moved, target), "the install follows the move")
	resolved, err = os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/local/demo"), resolved, "the symlink is adjusted to the new path")
	require.NoFileExists(t, filepath.Join(root, "skills/github/demo"))
}

// TestNormalizeActiveEntryRestoresLinksWhenMoveFails: if the actual move fails
// after the old links were uninstalled — the dry-run validation only rules out
// an occupied destination, not a move that cannot happen — the links are
// restored so a failed normalize never leaves the entry uninstalled at its old
// location.
func TestNormalizeActiveEntryRestoresLinksWhenMoveFails(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/github/demo/SKILL.md", frontmatter("demo", "a demo skill"))
	targetDir := t.TempDir()
	cfg := newCfg(root, []common.InstallTarget{{Name: "t", Path: targetDir, Kind: common.KindSkill}})
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	entry := findEntryByName(t, root, "demo")
	target, ok := svc.Installer.TargetByName("t")
	require.True(t, ok)
	tx := &dal.FileTransaction{}
	_, err = svc.Installer.Install(tx, entry, target, false)
	require.NoError(t, err)
	tx.Commit()
	link := filepath.Join(targetDir, "demo")
	require.Equal(t, common.InstallInstalled, svc.Installer.State(entry, target))

	// Make the destination's parent uncreatable: the dry-run existence check
	// passes (dest itself does not exist) but the move cannot create it.
	require.NoError(t, os.WriteFile(filepath.Join(root, "skills/local"), []byte("not a dir"), 0o644))

	_, err = svc.Normalize(context.Background(), "demo", "local", LifecycleOptions{})
	require.Error(t, err, "the move fails when the destination parent cannot be created")

	require.FileExists(t, filepath.Join(root, "skills/github/demo/SKILL.md"), "the entry did not move")
	require.Equal(t, common.InstallInstalled, svc.Installer.State(entry, target), "the failed move restores the old link")
	require.True(t, dal.IsSymlink(link), "the old link is back")
	resolved, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/github/demo"), resolved, "the restored link points at the old location")
}
