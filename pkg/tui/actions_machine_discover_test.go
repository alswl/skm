package tui

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachineDiscoverBatchRegistration(t *testing.T) {
	m := newTestModel(t)
	paths := []string{filepath.Join(t.TempDir(), "repo"), filepath.Join(t.TempDir(), "repo")}
	for _, path := range paths {
		output, err := exec.Command("git", "init", "-q", path).CombinedOutput()
		require.NoError(t, err, string(output))
	}
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	m.machine.sequence++
	m.acceptMachineDiscover(machineDiscoverMsg{sequence: m.machine.sequence, paths: paths})
	require.Len(t, m.picker.Items, 2)
	m.picker.OnConfirm(m.picker.Items)
	done := m.machinePending().(machineRegisterDoneMsg)
	require.NoError(t, done.err)
	dirs, err := m.svc.ListDirectories()
	require.NoError(t, err)
	require.Len(t, dirs, 2)
	require.Equal(t, "repo", dirs[0].ID)
	require.Equal(t, "repo-2", dirs[1].ID)
}
