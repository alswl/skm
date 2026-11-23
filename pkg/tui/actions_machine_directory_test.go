package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"

	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/services"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestMachineScanLatestRequestWins(t *testing.T) {
	m := &model{machine: machineDirectoryState{open: true, directories: []services.MachineDirectoryInfo{{ID: "a"}, {ID: "b"}}, cursor: 1}}
	require.NotNil(t, m.startMachineScan())
	first := m.machine.sequence
	m.machine.cursor = 2
	require.NotNil(t, m.startMachineScan())
	second := m.machine.sequence
	m.acceptMachineScan(machineScanMsg{sequence: second, scope: "b", items: []common.InstallationObservation{{Path: "b/skill"}}})
	m.acceptMachineScan(machineScanMsg{sequence: first, scope: "a", items: []common.InstallationObservation{{Path: "a/skill"}}})
	require.False(t, m.machine.scanning)
	require.Equal(t, "b/skill", m.machine.items[0].Path)
	require.NotNil(t, m.startMachineScan())
	m.acceptMachineScan(machineScanMsg{sequence: m.machine.sequence, scope: "b", err: errors.New("probe failed")})
	require.ErrorContains(t, m.machine.err, "probe failed")
	require.Empty(t, m.machine.items)
	m.machine.open = false
	m.acceptMachineScan(machineScanMsg{sequence: m.machine.sequence, scope: "b", items: []common.InstallationObservation{{Path: "late"}}})
	require.Empty(t, m.machine.items)
}

func TestMachineDirectoryTag(t *testing.T) {
	m := newTestModel(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, m.svc.RegisterDirectories(t.Context(), map[string]string{"project": project}, nil))
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	scan = m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	require.NotNil(t, scan)
	m.acceptMachineScan(scan().(machineScanMsg))
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("team")})
	cmd := m.handleMachineKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	done := cmd().(machineTagDoneMsg)
	require.NoError(t, done.err)
	m.acceptMachineTagDone(done)
	list := m.machinePending().(machineDirectoriesMsg)
	scan = m.acceptMachineDirectories(list)
	require.Equal(t, "project", m.machineScope())
	m.acceptMachineScan(scan().(machineScanMsg))
	require.Equal(t, []string{"team"}, m.machine.directories[0].Tags)
}

func TestMachineDirectoryBatchTagAndFilter(t *testing.T) {
	m := newTestModel(t)
	for _, id := range []string{"a", "b"} {
		project := filepath.Join(t.TempDir(), id)
		output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
		require.NoError(t, err, string(output))
		require.NoError(t, m.svc.RegisterDirectories(t.Context(), map[string]string{id: project}, nil))
	}
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	m.openMachineBatchTag()
	require.Len(t, m.picker.Items, 2)
	m.picker.OnConfirm(m.picker.Items)
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("team")})
	cmd := m.handleMachineKey(tea.KeyMsg{Type: tea.KeyEnter})
	done := cmd().(machineTagDoneMsg)
	require.NoError(t, done.err)
	m.acceptMachineTagDone(done)
	scan = m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("team")})
	scan = m.handleMachineKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Len(t, m.machine.directories, 2)
	m.acceptMachineScan(scan().(machineScanMsg))
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m.machine.filterInput = "missing"
	scan = m.handleMachineKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.Empty(t, m.machine.directories)
	require.Len(t, m.machine.allDirectories, 2)
	require.NotNil(t, scan)
}

func TestMachineDirectoryLateScanAndCaptureOnlySelectedScope(t *testing.T) {
	m := newTestModel(t)
	projects := map[string]string{}
	for _, id := range []string{"a", "b"} {
		project := filepath.Join(t.TempDir(), id)
		output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
		require.NoError(t, err, string(output))
		require.NoError(t, m.svc.RegisterDirectories(t.Context(), map[string]string{id: project}, []string{"team"}))
		projects[id] = project
	}
	source := filepath.Join(m.svc.Cfg.Root, "skills", "local", "skill-a")
	for id, client := range map[string]string{"a": ".agents", "b": ".claude"} {
		path := filepath.Join(projects[id], client, "skills", "skill-a")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.Symlink(source, path))
	}
	m.openMachineDirectories()
	_ = m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.machine.cursor = 1
	first := m.startMachineScan()
	m.machine.cursor = 2
	second := m.startMachineScan()
	m.acceptMachineScan(second().(machineScanMsg))
	m.acceptMachineScan(first().(machineScanMsg))
	require.Len(t, m.machine.items, 1)
	require.Equal(t, "b", m.machine.items[0].Directory)
	require.Equal(t, "claude", m.machine.items[0].Target)
	m.openMachineCapturePicker()
	m.picker.OnConfirm(m.picker.Items[:1])
	m.acceptMachineCapturePlan(m.machinePending().(machineCapturePlanMsg))
	require.NotNil(t, m.confirm)
	m.confirm.OnYes()
	done := m.machinePending().(machineCaptureDoneMsg)
	require.NoError(t, done.err)
	doc, err := config.LoadInstallations(filepath.Join(m.svc.Cfg.Root, "installations.yaml"))
	require.NoError(t, err)
	require.Len(t, doc.Installs, 1)
	require.Equal(t, "b", doc.Installs[0].Scope.Name)
	require.Equal(t, "claude", doc.Installs[0].Target)
}

func TestMachineDirectoryScanViewStates(t *testing.T) {
	m := newTestModel(t)
	m.machine.open = true
	m.machine.loadingList = true
	require.Contains(t, m.machineView(), "Loading directories")
	m.machine.loadingList = false
	m.machine.scanning = true
	require.Contains(t, m.machineView(), "Scanning")
	m.machine.scanning = false
	require.Contains(t, m.machineView(), "No skills found")
	m.machine.err = errors.New("permission denied")
	require.Contains(t, m.machineView(), "permission denied")
}

func TestMachineDirectoryCaptureUser(t *testing.T) {
	m := newTestModel(t)
	var target string
	for _, entry := range m.svc.Cfg.Targets {
		if entry.Name == "t" {
			target = entry.Path
		}
	}
	require.NotEmpty(t, target)
	link := filepath.Join(target, "skill-a")
	require.NoError(t, os.Symlink(filepath.Join(m.svc.Cfg.Root, "skills", "local", "skill-a"), link))
	m.openMachineDirectories()
	listMsg := m.machinePending().(machineDirectoriesMsg)
	scanCmd := m.acceptMachineDirectories(listMsg)
	require.NotNil(t, scanCmd)
	m.acceptMachineScan(scanCmd().(machineScanMsg))
	m.openMachineCapturePicker()
	require.NotNil(t, m.picker)
	picker := m.picker
	picker.OnConfirm(picker.Items[:1])
	msg := m.machinePending().(machineCapturePlanMsg)
	m.acceptMachineCapturePlan(msg)
	require.NotNil(t, m.confirm)
	m.confirm.OnYes()
	done := m.machinePending().(machineCaptureDoneMsg)
	require.NoError(t, done.err)
	require.NotNil(t, m.acceptMachineCaptureDone(done))
	doc, err := config.LoadInstallations(filepath.Join(m.svc.Cfg.Root, "installations.yaml"))
	require.NoError(t, err)
	require.Len(t, doc.Installs, 1)
	require.Equal(t, "user", doc.Installs[0].Scope.Kind)
	require.FileExists(t, link)
}

func TestMachineRepreviewRequestedWhileScanning(t *testing.T) {
	m := newTestModel(t)
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	require.NotNil(t, scan)
	called := false
	m.machine.retryPreview = func() { called = true }
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	require.True(t, m.machine.retryPreviewPending)
	require.False(t, called)
	m.acceptMachineScan(scan().(machineScanMsg))
	require.True(t, called)
	require.False(t, m.machine.retryPreviewPending)
}
