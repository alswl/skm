package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// The address the user just typed usually settles both questions the import
// flow asks. A GitHub blob URL ending in SKILL.md can only be handled by the
// github provider and can only be a skill, yet both pickers used to open on
// "auto" and make the user walk past the answer. They now open already on it.

// typeImport drives the import prompt: "m", the address, then Enter.
func typeImport(t *testing.T, m *model, addr string) {
	t.Helper()
	m.handleKey(runeKey('m'))
	require.True(t, m.importing, "the import prompt is open")
	for _, r := range addr {
		m.handleKey(runeKey(r))
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
}

// currentLabel is the label the picker's cursor is sitting on.
func currentLabel(t *testing.T, m *model) string {
	t.Helper()
	require.NotNil(t, m.picker)
	require.Less(t, m.picker.Cursor, len(m.picker.Items))
	return m.picker.Items[m.picker.Cursor].Label
}

func TestImportPreselectsTheProviderTheAddressImplies(t *testing.T) {
	m := newTestModel(t)
	typeImport(t, &m, "https://github.com/alswl/mind-forge/blob/master/skills/mf-cli/SKILL.md")

	label := currentLabel(t, &m)
	require.True(t, strings.HasPrefix(label, "github"),
		"the provider picker opens on github, not auto: %q", label)
	require.Contains(t, label, "detected", "the pre-selection says why it was made: %q", label)
}

func TestImportPreselectsLocalProviderForAnExistingExternalSkill(t *testing.T) {
	m := newTestModel(t)
	src := filepath.Join(t.TempDir(), "d2")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: d2\ndescription: diagram\n---\nbody\n"), 0o644))

	typeImport(t, &m, src)
	require.True(t, strings.HasPrefix(currentLabel(t, &m), "local"), "existing external directories must preselect local")

	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, strings.HasPrefix(currentLabel(t, &m), "auto"), "a directory still uses probe-based kind detection")
}

func TestImportPreselectsLocalProviderAndSkillKindForLocalSkillMarker(t *testing.T) {
	m := newTestModel(t)
	src := filepath.Join(t.TempDir(), "d2")
	require.NoError(t, os.MkdirAll(src, 0o755))
	marker := filepath.Join(src, "SKILL.md")
	require.NoError(t, os.WriteFile(marker, []byte("---\nname: d2\ndescription: diagram\n---\nbody\n"), 0o644))

	typeImport(t, &m, marker)
	require.True(t, strings.HasPrefix(currentLabel(t, &m), "local"), "an existing local marker must preselect local")

	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, strings.HasPrefix(currentLabel(t, &m), "skill"), "SKILL.md must preselect skill")
}

func TestImportExistingExternalLocalSkillCompletesThroughTUI(t *testing.T) {
	for _, markerAddress := range []bool{false, true} {
		t.Run(map[bool]string{false: "directory", true: "skill marker"}[markerAddress], func(t *testing.T) {
			m := newTestModel(t)
			src := filepath.Join(t.TempDir(), "d2")
			require.NoError(t, os.MkdirAll(src, 0o755))
			marker := filepath.Join(src, "SKILL.md")
			require.NoError(t, os.WriteFile(marker, []byte("---\nname: d2\ndescription: diagram\n---\nbody\n"), 0o644))

			addr := src
			if markerAddress {
				addr = marker
			}
			typeImport(t, &m, addr)
			m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // local provider
			m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // inferred or auto kind
			drainJob(t, &m)

			entry := m.svc.FindEntry("d2")
			require.NotNil(t, entry)
			require.FileExists(t, filepath.Join(entry.Path, "SKILL.md"))
			require.FileExists(t, marker, "TUI import must leave the external source intact")
		})
	}
}

func TestImportPreselectsTheKindTheAddressImplies(t *testing.T) {
	cases := []struct {
		addr string
		want string
	}{
		{"https://github.com/o/r/blob/main/skills/x/SKILL.md", "skill"},
		{"https://github.com/o/r/blob/main/commands/x/command.md", "command"},
		{"https://github.com/o/r", "auto"},
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			m := newTestModel(t)
			typeImport(t, &m, tc.addr)
			require.NotNil(t, m.picker, "provider picker")
			m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // accept the provider

			label := currentLabel(t, &m)
			require.True(t, strings.HasPrefix(label, tc.want),
				"the kind picker opens on %q: %q", tc.want, label)
		})
	}
}

// An address no provider claims has nothing to infer from, so the flow must
// still open on "auto" rather than guessing.
func TestImportFallsBackToAutoWhenNothingMatches(t *testing.T) {
	m := newTestModel(t)
	typeImport(t, &m, "https://codeberg.org/o/r/src/branch/main/x")
	require.True(t, strings.HasPrefix(currentLabel(t, &m), "auto"),
		"unmatched address leaves the provider picker on auto")
}

// Pre-selecting must not take the choice away: every provider is still listed
// and the cursor can be moved off the detected one.
func TestImportStillOffersEveryProvider(t *testing.T) {
	m := newTestModel(t)
	typeImport(t, &m, "https://github.com/o/r/blob/main/x/SKILL.md")
	require.NotNil(t, m.picker)

	var labels []string
	for _, it := range m.picker.Items {
		labels = append(labels, it.Label)
	}
	require.Len(t, labels, len(m.svc.Registry.Providers())+1, "auto plus every provider")
	require.True(t, strings.HasPrefix(labels[0], "auto"), "auto stays first: %q", labels[0])
}
