package services

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

// TestFindEntryLocatesByRepoPath: an entry's identity is its repo-relative
// path, not its name (data-model.md). A same-named active/archived pair must
// be addressable by path — "skills/.../demo" → the active copy,
// "archived/.../demo" → the archived copy — while a bare name keeps resolving
// via the global name space (active copy first, scan order).
func TestFindEntryLocatesByRepoPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "new active version"))
	writeFile(t, root, "archived/local/demo/SKILL.md", frontmatter("demo", "old archived version"))

	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{}}, common.NewLogger(false))
	require.NoError(t, err)

	active := svc.FindEntry("skills/local/demo")
	require.NotNil(t, active, "path lookup resolves the active copy")
	require.Equal(t, common.StatusActive, active.Status)
	require.Equal(t, filepath.Join(root, "skills", "local", "demo"), active.Path)

	archived := svc.FindEntry("archived/local/demo")
	require.NotNil(t, archived, "path lookup resolves the archived copy, not the active one")
	require.Equal(t, common.StatusArchived, archived.Status)
	require.Equal(t, filepath.Join(root, "archived", "local", "demo"), archived.Path)

	// A bare name still resolves via the global name space (active first).
	byName := svc.FindEntry("demo")
	require.NotNil(t, byName)
	require.Equal(t, common.StatusActive, byName.Status, "bare name resolves to the active copy by scan order")
	require.Equal(t, active.Path, byName.Path)

	// A nonexistent path does not resolve and does not fall through to a
	// same-named entry at another path.
	require.Nil(t, svc.FindEntry("archived/local/demo2"))
	require.Nil(t, svc.FindEntry(""))
}

func TestResolveEntryRejectsSameTierNameAmbiguity(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/github/team/demo/SKILL.md", frontmatter("demo", "remote"))
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "local"))
	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{}}, common.NewLogger(false))
	require.NoError(t, err)

	entry, err := svc.ResolveEntry("demo")
	require.Nil(t, entry)
	require.Error(t, err)
	require.Equal(t, common.ExitObject, common.ExitCodeOf(err, common.ExitError))
	require.Contains(t, err.Error(), "skills/github/team/demo")
	require.Contains(t, err.Error(), "skills/local/demo")
	require.True(t, strings.Contains(err.Error(), "re-run with one of the paths"))

	entry, err = svc.ResolveEntry("skills/local/demo")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/local/demo"), entry.Path)
}

func TestResolveEntryUsesActiveOverArchivedAndNeverFallsBackFromPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "active"))
	writeFile(t, root, "archived/local/demo/SKILL.md", frontmatter("demo", "archived"))
	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{}}, common.NewLogger(false))
	require.NoError(t, err)

	entry, err := svc.ResolveEntry("demo")
	require.NoError(t, err)
	require.Equal(t, common.StatusActive, entry.Status)
	entry, err = svc.ResolveEntry("archived/local/missing")
	require.NoError(t, err)
	require.Nil(t, entry)
}
