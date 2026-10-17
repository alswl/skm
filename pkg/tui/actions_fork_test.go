package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
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

// F forks and f fixes installs, from both the list and the detail page. F
// never replaces or discards an existing fork: that stays behind the menu's
// confirmation.
func TestForkAndFixKeys(t *testing.T) {
	m := newTestModel(t)
	selectEntry(t, &m, "skill-a")
	forkDir := filepath.Join(m.svc.Cfg.Root, "skills-fork", "local", "skill-a")

	m.handleListKey(runeKey('f'))
	drainJob(t, &m)
	require.Contains(t, m.status, "fix")
	require.NoDirExists(t, forkDir, "f is fix, not fork")

	m.handleListKey(runeKey('F'))
	drainJob(t, &m)
	require.Contains(t, m.status, "forked")
	require.DirExists(t, forkDir)

	selectEntry(t, &m, "skill-a")
	require.NoError(t, os.WriteFile(filepath.Join(forkDir, "mine.txt"), []byte("x"), 0o644))
	m.openDetail()
	var forkHint bool
	for _, b := range m.detailBindings() {
		if b.Keys == "F" {
			forkHint = true
			require.False(t, b.Enabled, "F is dimmed once a fork exists")
		}
	}
	require.True(t, forkHint, "the detail footer shows F")
	m.handleDetailKey(runeKey('F'))
	require.Nil(t, m.confirm)
	require.Contains(t, m.status, "already has a fork")
	require.FileExists(t, filepath.Join(forkDir, "mine.txt"), "pressing F again keeps the user's edits")
}

// A forked entry is visibly marked in the list: its status column reads
// active+fork, the selection summary says so, and discarding the fork removes
// the mark.
func TestListMarksForkedEntries(t *testing.T) {
	m := newTestModel(t)
	selectEntry(t, &m, "skill-a")
	rowOf := func(name string) string {
		for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.Contains(line, name) {
				return line
			}
		}
		t.Fatalf("no row for %q in:\n%s", name, ansi.Strip(m.View()))
		return ""
	}
	require.NotContains(t, rowOf("skill-a"), "+fork")
	require.NotContains(t, m.selectionSummary(), "+fork")

	menuAction(t, &m, "create fork").Run()
	drainJob(t, &m)
	selectEntry(t, &m, "skill-a")
	require.Contains(t, rowOf("skill-a"), "active+fork")
	require.NotContains(t, rowOf("skill-b"), "+fork", "only the forked entry is marked")
	require.Contains(t, m.selectionSummary(), "active+fork")

	menuAction(t, &m, "discard fork").Run()
	_ = m.handleConfirmKey(runeKey('y'))
	drainJob(t, &m)
	selectEntry(t, &m, "skill-a")
	require.NotContains(t, rowOf("skill-a"), "+fork")

	m.showHelp = true
	require.Contains(t, ansi.Strip(m.View()), "+fork")
}

// A fork takes effect at once: the installs follow it inside the same
// operation, and the list shows the new state without a manual refresh or a
// second keypress.
func TestForkTakesEffectImmediately(t *testing.T) {
	m := newTestModel(t)
	_, err := m.svc.Install(t.Context(), "skill-a", services.InstallOptions{Targets: []string{"t"}})
	require.NoError(t, err)
	m.applyScan(m.svc.Scan())
	selectEntry(t, &m, "skill-a")
	installedCell := func() common.InstallState {
		e := m.filtered[m.cursor]
		for _, c := range m.installs[e.Path] {
			if c.name == "t" {
				return c.state
			}
		}
		return ""
	}
	require.Equal(t, common.InstallInstalled, installedCell())

	menuAction(t, &m, "create fork").Run()
	drainJob(t, &m)

	e := m.filtered[m.cursor]
	require.Equal(t, "skill-a", e.Name, "the cursor stays on the forked entry")
	require.NotEmpty(t, e.UpstreamPath, "the list already shows the fork")
	require.Equal(t, common.InstallInstalled, installedCell(), "the install column is up to date for the fork's path")
	target, ok := m.svc.Installer.TargetByName("t")
	require.True(t, ok)
	link, err := os.Readlink(filepath.Join(target.Path, "skill-a"))
	require.NoError(t, err)
	require.Equal(t, e.Path, link, "the target already serves the fork")
}
