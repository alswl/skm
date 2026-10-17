package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func menuAction(t *testing.T, m *model, labelPrefix string) entryAction {
	t.Helper()
	for _, a := range m.entryActions() {
		if strings.HasPrefix(a.Label, labelPrefix) {
			return a
		}
	}
	t.Fatalf("no menu action %q in %v", labelPrefix, m.entryActions())
	return entryAction{}
}

func TestForkMenuCreatesReplacesAndDiscards(t *testing.T) {
	m := newTestModel(t)
	selectEntry(t, &m, "skill-a")
	for _, a := range m.entryActions() {
		require.NotContains(t, a.Label, "discard", "no fork yet")
	}

	menuAction(t, &m, "create fork").Run()
	drainJob(t, &m)
	require.Contains(t, m.status, "forked")
	e := m.svc.FindEntry("skill-a")
	require.Equal(t, filepath.Join(m.svc.Cfg.Root, "skills-fork", "local", "skill-a"), e.Path)
	require.NotEmpty(t, e.UpstreamPath)

	selectEntry(t, &m, "skill-a")
	require.Contains(t, m.buildDetail(), "fork:")
	require.NoError(t, os.WriteFile(filepath.Join(e.Path, "mine.txt"), []byte("x"), 0o644))
	menuAction(t, &m, "replace fork").Run()
	require.NotNil(t, m.confirm, "replacing deletes the user's copy, so it asks first")
	_ = m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	require.NoFileExists(t, filepath.Join(e.Path, "mine.txt"))

	selectEntry(t, &m, "skill-a")
	menuAction(t, &m, "discard fork").Run()
	require.NotNil(t, m.confirm)
	_ = m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	require.NoDirExists(t, filepath.Join(m.svc.Cfg.Root, "skills-fork", "local", "skill-a"))
	require.Empty(t, m.svc.FindEntry("skill-a").UpstreamPath)
}

func TestArchiveAndDeleteAreRefusedWhileForked(t *testing.T) {
	m := newTestModel(t)
	selectEntry(t, &m, "skill-a")
	menuAction(t, &m, "create fork").Run()
	drainJob(t, &m)
	selectEntry(t, &m, "skill-a")

	m.archiveSelected()
	require.Contains(t, m.status, "discard the fork first")
	require.Nil(t, m.confirm)
	m.deleteSelected()
	require.Contains(t, m.status, "discard the fork first")
	require.Nil(t, m.confirm)
	require.DirExists(t, filepath.Join(m.svc.Cfg.Root, "skills", "local", "skill-a"))
}

func TestUpdateSelectedShowsForkConflict(t *testing.T) {
	m := newTestModel(t)
	src := t.TempDir()
	body := "line1\nline2\nline3\nline4\nline5\n"
	writeFileT(t, src, "SKILL.md", "---\nname: forked\ndescription: forked skill\n---\n"+body)
	m.runImport(src, "", "auto")
	drainJob(t, &m)
	selectEntry(t, &m, "forked")
	menuAction(t, &m, "create fork").Run()
	drainJob(t, &m)
	selectEntry(t, &m, "forked")
	e := m.filtered[m.cursor]
	writeFileT(t, e.Path, "SKILL.md", "---\nname: forked\ndescription: forked skill\n---\nmine\nline2\nline3\nline4\nline5\n")
	writeFileT(t, src, "SKILL.md", "---\nname: forked\ndescription: forked skill\n---\ntheirs\nline2\nline3\nline4\nline5\n")
	before, err := os.ReadFile(filepath.Join(e.Path, "SKILL.md"))
	require.NoError(t, err)

	m.updateSelected()
	drainJob(t, &m)
	require.Contains(t, m.status, "conflict in SKILL.md")
	after, err := os.ReadFile(filepath.Join(e.Path, "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
}
