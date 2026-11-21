package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestMachineDropConfirmationKeepsLink(t *testing.T) {
	m := newTestModel(t)
	var target string
	for _, item := range m.svc.Cfg.Targets {
		if item.Name == "t" {
			target = item.Path
		}
	}
	link := filepath.Join(target, "skill-a")
	require.NoError(t, os.Symlink(filepath.Join(m.svc.Cfg.Root, "skills", "local", "skill-a"), link))
	plan, err := m.svc.PlanMachineCapture("", "t", link)
	require.NoError(t, err)
	require.NoError(t, m.svc.ApplyMachineCapture(t.Context(), *plan))
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	m.acceptMachineDeclarations(m.startMachineDrop()().(machineDeclarationsMsg))
	require.NotNil(t, m.picker)
	m.picker.OnConfirm(m.picker.Items[:1])
	m.acceptMachineDropPlan(m.machinePending().(machineDropPlanMsg))
	require.Contains(t, m.confirm.Prompt, "Links stay in place")
	m.confirm.OnYes()
	done := m.machinePending().(machineDropDoneMsg)
	require.NoError(t, done.err)
	doc, err := config.LoadMachineDocument(filepath.Join(m.svc.Cfg.Root, "machine.yaml"))
	require.NoError(t, err)
	require.Empty(t, doc.Installs)
	_, err = os.Readlink(link)
	require.NoError(t, err)
}
