package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestMachineRestoreConfirmation(t *testing.T) {
	m := newTestModel(t)
	var target string
	for _, item := range m.svc.Cfg.Targets {
		if item.Name == "t" {
			target = item.Path
		}
	}
	link := filepath.Join(target, "skill-a")
	skill := filepath.Join(m.svc.Cfg.Root, "skills", "local", "skill-a")
	require.NoError(t, os.Symlink(skill, link))
	capture, err := m.svc.PlanMachineCapture("", "t", link)
	require.NoError(t, err)
	require.NoError(t, m.svc.ApplyMachineCapture(t.Context(), *capture))
	require.NoError(t, os.Remove(link))
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	m.openMachineRestore()
	m.picker.OnConfirm(m.picker.Items[:1])
	preview := m.machinePending().(machineRestorePlanMsg)
	require.NoError(t, preview.err)
	m.acceptMachineRestorePlan(preview)
	require.Contains(t, m.confirm.Prompt, "Restore 1 locations")
	require.NotContains(t, m.confirm.Prompt, link)
	m.confirm.OnYes()
	done := m.machinePending().(machineRestoreDoneMsg)
	require.NoError(t, done.err)
	require.Equal(t, "installed", done.result.Items[0].Status)
	got, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, skill, got)
}

func TestMachineRestoreStaleFeedbackAndRepreview(t *testing.T) {
	m := newTestModel(t)
	var target string
	for _, item := range m.svc.Cfg.Targets {
		if item.Name == "t" {
			target = item.Path
		}
	}
	link := filepath.Join(target, "skill-a")
	skill := filepath.Join(m.svc.Cfg.Root, "skills", "local", "skill-a")
	require.NoError(t, os.Symlink(skill, link))
	capture, err := m.svc.PlanMachineCapture("", "t", link)
	require.NoError(t, err)
	require.NoError(t, m.svc.ApplyMachineCapture(t.Context(), *capture))
	require.NoError(t, os.Remove(link))
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	m.openMachineRestore()
	m.picker.OnConfirm(m.picker.Items[:1])
	preview := m.machinePending().(machineRestorePlanMsg)
	require.NoError(t, preview.err)
	m.acceptMachineRestorePlan(preview)
	require.NoError(t, os.WriteFile(link, []byte("outside"), 0o644))
	m.confirm.OnYes()
	done := m.machinePending().(machineRestoreDoneMsg)
	require.ErrorContains(t, done.err, "stale")
	m.acceptMachineRestoreDone(done)
	require.Contains(t, m.machineView(), "preview again")
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	refreshed := m.machinePending().(machineRestorePlanMsg)
	require.NoError(t, refreshed.err)
	require.Equal(t, "conflict", refreshed.plan.Plans[0].Items[0].Status)
	data, err := os.ReadFile(link)
	require.NoError(t, err)
	require.Equal(t, "outside", string(data))
}
