package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestDirectoryScanOpensSelectedResultAndReturnsToItsRow(t *testing.T) {
	m := newTestModel(t)
	m.machine.open = true
	m.machine.items = []common.InstallationObservation{
		{Target: "codex", Skill: "skills/local/skill-a", Status: "installed", Path: "/tmp/agents/skill-a"},
		{Target: "claude", Skill: "skills/local/skill-b", Status: "installed", Path: "/tmp/claude/skill-b"},
	}
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyTab})
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyDown})
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyEnter})
	m.acceptLocationCoverage(m.machinePending().(locationCoverageMsg))
	require.Contains(t, m.picker.Prelude, "/tmp/claude/skill-b")
	require.Contains(t, m.picker.Title, "claude")
	m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEsc})
	require.Nil(t, m.picker)
	require.True(t, m.machine.resultFocused)
	require.Equal(t, 1, m.machine.resultCursor)
}

func TestMachineRestoreReturnsToPreviewAfterBindingMissingPath(t *testing.T) {
	m := newTestModel(t)
	project := filepath.Join(t.TempDir(), "project")
	out, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(out))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".gitignore"), []byte("/.agents/skills/\n"), 0o600))
	doc := config.InstallationsDocument{
		Version:     1,
		Directories: map[string]common.Directory{"project": {Tags: []string{"team"}}},
		Installs: []common.Installation{{
			Skill: "skills/local/skill-a", Target: "codex",
			Scope: common.InstallationScope{Kind: "tag", Name: "team"},
		}},
	}
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(m.svc.Cfg.Root, config.InstallationsFile), doc, 0o644))
	m.openMachineDirectories()
	m.acceptMachineScan(m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))().(machineScanMsg))
	m.openMachineRestore()
	m.acceptMachineRestorePlan(m.machinePending().(machineRestorePlanMsg))
	require.Contains(t, m.confirm.Prompt, "Set local path for: project")
	m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	m.handleMachinePathInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(project)})
	done := m.handleMachinePathInput(tea.KeyMsg{Type: tea.KeyEnter})().(machinePathDoneMsg)
	require.NoError(t, done.err)
	m.acceptMachinePathDone(done)
	m.acceptMachineRestorePlan(m.machinePending().(machineRestorePlanMsg))
	require.Nil(t, m.confirm.OnSetPath)
	require.Contains(t, m.confirm.Prompt, "Restore 1 locations")
	link := filepath.Join(project, ".agents", "skills", "skill-a")
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err))
	m.confirm.OnYes()
	applied := m.machinePending().(machineRestoreDoneMsg)
	require.NoError(t, applied.err)
	_, err = os.Readlink(link)
	require.NoError(t, err)
}
