package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestArchiveUnarchivePreservesLayout(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/team/demo/SKILL.md", frontmatter("demo", "a skill"))
	repo := New(root)
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

	_, err := New(root).Archive(context.Background(), entry, LifecycleOptions{DryRun: true})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "skills/local/demo/SKILL.md"))
	require.NoDirExists(t, filepath.Join(root, "archived"))
}

func TestDeleteRequiresForce(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "a skill"))
	entry := findEntryByName(t, root, "demo")

	err := New(root).Delete(context.Background(), entry, LifecycleOptions{})
	require.Error(t, err, "delete without --force must be refused")
	require.FileExists(t, filepath.Join(root, "skills/local/demo/SKILL.md"))

	require.NoError(t, New(root).Delete(context.Background(), entry, LifecycleOptions{Force: true}))
	require.NoDirExists(t, filepath.Join(root, "skills/local/demo"))
}

func TestConvertFlipsKindAndDropsOrigin(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "a skill"))
	writeFile(t, root, "skills/local/demo/meta.json", `{"address":"https://x","mode_id":"local"}`)
	entry := findEntryByName(t, root, "demo")

	newEntry, err := New(root).ConvertContent(context.Background(), entry, common.KindCommand)
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
	_, err := New(root).ConvertContent(context.Background(), single, common.KindSkill)
	require.Error(t, err)
	require.Contains(t, err.Error(), "single-file")

	// Archived entry cannot be converted.
	writeFile(t, root, "archived/local/old/SKILL.md", frontmatter("old", "archived"))
	old := findEntryByName(t, root, "old")
	_, err = New(root).ConvertContent(context.Background(), old, common.KindCommand)
	require.Error(t, err)
	require.FileExists(t, filepath.Join(root, "archived/local/old/SKILL.md"), "no half-state")
}
