package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachineUnlinkConfirmationRechecks(t *testing.T) {
	m := newTestModel(t)
	var target string
	for _, entry := range m.svc.Cfg.Targets {
		if entry.Name == "t" {
			target = entry.Path
		}
	}
	link := filepath.Join(target, "skill-a")
	skill := filepath.Join(m.svc.Cfg.Root, "skills", "local", "skill-a")
	require.NoError(t, os.Symlink(skill, link))
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	m.openMachineUnlinkPicker()
	require.NotNil(t, m.picker)
	m.picker.OnConfirm(m.picker.Items[:1])
	preview := m.machinePending().(machineUnlinkPlanMsg)
	require.NoError(t, preview.err)
	m.acceptMachineUnlinkPlan(preview)
	require.Contains(t, m.confirm.Prompt, link)
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.WriteFile(link, []byte("external"), 0o644))
	m.confirm.OnYes()
	result := m.machinePending().(machineUnlinkDoneMsg)
	require.Error(t, result.err)
	data, err := os.ReadFile(link)
	require.NoError(t, err)
	require.Equal(t, "external", string(data))
}
