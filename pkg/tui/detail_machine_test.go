package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
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

	m.openDetailInstallations()
	require.NotNil(t, m.picker)
	var position int
	for i, item := range m.picker.Items {
		if item.Label == "project / codex · installed" {
			position = i
			break
		}
	}
	require.Equal(t, "project / codex · installed", m.picker.Items[position].Label)
	m.picker.OnConfirm(m.picker.Items[position : position+1])
	m.acceptLocationCoverage(m.machinePending().(locationCoverageMsg))
	require.Equal(t, "unlink", m.picker.Items[1].Value)
	m.picker.OnConfirm(m.picker.Items[1:2])
	preview := m.machinePending().(locationPreviewMsg)
	require.NoError(t, preview.err)
	m.picker = nil
	m.acceptLocationPreview(preview)
	require.Contains(t, m.confirm.Prompt, "codex")
	m.confirm.OnYes()
	result := m.machinePending().(locationAppliedMsg)
	require.NoError(t, result.err)
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err))
}

func TestDetailLocationsRequestedDuringScanOpenWhenReady(t *testing.T) {
	m := newTestModel(t)
	m.openDetail()
	require.True(t, m.detailMachineLoading)
	m.openDetailInstallations()
	require.True(t, m.detailOpenLocationsPending)
	msg := m.machinePending().(machineSkillScanMsg)
	require.NoError(t, msg.err)
	msg.items = []common.InstallationObservation{{Directory: "project", Target: "codex", Status: "installed"}}
	m.acceptMachineSkillScan(msg)
	require.False(t, m.detailOpenLocationsPending)
	require.False(t, m.detailMachineLoading)
	require.NotNil(t, m.picker)
	require.Equal(t, "project / codex · installed", m.picker.Items[0].Label)
}
