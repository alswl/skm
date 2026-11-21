package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachineSkillDetailFindsDirectoryAfterTagRemoval(t *testing.T) {
	m := newTestModel(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, m.svc.RegisterDirectories(t.Context(), map[string]string{"project": project}, []string{"team"}))
	link := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(m.svc.Cfg.Root, "skills", "local", "skill-a"), link))
	require.NoError(t, m.svc.TagDirectories(t.Context(), []string{"project"}, nil, []string{"team"}))
	for i, entry := range m.filtered {
		if entry.Name == "skill-a" {
			m.cursor = i
			break
		}
	}
	m.openDetail()
	require.True(t, m.detailMachineLoading)
	msg := m.machinePending().(machineSkillScanMsg)
	m.acceptMachineSkillScan(msg)
	require.False(t, m.detailMachineLoading)
	require.Contains(t, m.detailContent(), link)
	require.Contains(t, m.detailContent(), "project")
}
