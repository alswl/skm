package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// T022: add/edit/remove flow, field navigation, validation-failure keeps the
// form open with a reason.

func typeText(m *model, s string) {
	for _, r := range s {
		_ = m.handleKey(runeKey(r))
	}
}

func TestTargetEditorAddFlow(t *testing.T) {
	m := newTestModel(t)
	before := len(m.svc.Cfg.Targets)

	_ = m.handleKey(runeKey('t')) // open target editor
	require.True(t, m.showTargets)
	require.Contains(t, m.View(), "targets")

	_ = m.handleKey(runeKey('a')) // add
	require.NotNil(t, m.targetWizard)
	require.False(t, m.targetWizard.editing)

	typeText(&m, "my-tool")
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // name -> platform
	require.Equal(t, wizardStepPlatform, m.targetWizard.step)

	typeText(&m, "mytool")
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // platform -> path
	require.Equal(t, wizardStepPath, m.targetWizard.step)

	typeText(&m, m.svc.Cfg.Root+"/../mytool-target")
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // path -> accepts picker
	require.Nil(t, m.targetWizard)
	require.NotNil(t, m.picker, "path entry opens the accepts picker")
	require.Contains(t, m.picker.title, "accepts")

	// Toggle both skill and command on, then confirm.
	_ = m.handlePickerKey(runeKey(' ')) // check skill (cursor starts at 0)
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyDown})
	_ = m.handlePickerKey(runeKey(' ')) // check command
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})

	// skill auto-resolves (only one compatible strategy); command needs a pick.
	require.NotNil(t, m.picker, "command strategy needs an explicit choice")
	require.Contains(t, m.picker.title, "command")
	_ = m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter}) // pick the first (command-marker)

	drainJob(t, &m)
	require.Contains(t, m.status, "added target my-tool")
	require.Len(t, m.svc.Cfg.Targets, before+1)
}

func TestTargetEditorEditPathFlow(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('t'))
	require.NotEmpty(t, m.svc.Cfg.Targets, "fixture seeds one target")

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // edit selected target's path
	require.NotNil(t, m.targetWizard)
	require.True(t, m.targetWizard.editing)
	name := m.targetWizard.draft.Name

	// Clear the pre-filled path and type a new one.
	for len(m.targetWizard.text) > 0 {
		_ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	typeText(&m, m.svc.Cfg.Root+"/../new-path")
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, m.targetWizard)

	drainJob(t, &m)
	require.Contains(t, m.status, "updated target "+name)
}

func TestTargetEditorAddRejectsEmptyName(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('t'))
	_ = m.handleKey(runeKey('a'))
	require.NotNil(t, m.targetWizard)

	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}) // Enter with empty name
	require.NotNil(t, m.targetWizard, "the form stays open on validation failure")
	require.Equal(t, wizardStepName, m.targetWizard.step)
	require.Contains(t, m.status, "name must be non-empty")
}

func TestTargetEditorRemoveFlow(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('t'))
	name := m.svc.Cfg.Targets[m.targetsCursor].Name

	_ = m.handleKey(runeKey('d'))
	require.NotNil(t, m.confirm, "remove asks for confirmation")

	_ = m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyEnter})
	drainJob(t, &m)
	require.Contains(t, m.status, "removed target "+name)
	for _, tgt := range m.svc.Cfg.Targets {
		require.NotEqual(t, name, tgt.Name)
	}
}

func TestTargetEditorEscReturnsToList(t *testing.T) {
	m := newTestModel(t)
	_ = m.handleKey(runeKey('t'))
	require.True(t, m.showTargets)
	_ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	require.False(t, m.showTargets)
}
