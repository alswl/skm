package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/services"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestMachineInstallByOverlappingTags(t *testing.T) {
	m := newTestModel(t)
	projects := map[string]string{}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("d%02d", i)
		project := filepath.Join(t.TempDir(), id)
		out, err := exec.Command("git", "init", "-q", project).CombinedOutput()
		require.NoError(t, err, string(out))
		var tags []string
		if i < 15 {
			tags = append(tags, "team")
		}
		if i >= 5 {
			tags = append(tags, "extra")
		}
		require.NoError(t, m.svc.RegisterDirectories(t.Context(), map[string]string{id: project}, tags))
		projects[id] = project
	}
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	m.openMachineInstallSkills()
	require.NotNil(t, m.picker)
	require.Len(t, m.picker.Items, 2)
	m.picker.OnConfirm(m.picker.Items)
	require.Equal(t, "installation scope", m.picker.Title)
	m.picker.OnConfirm(m.picker.Items[1:2])
	require.Equal(t, "select tags", m.picker.Title)
	m.picker.OnConfirm(m.picker.Items)
	require.Equal(t, "select clients", m.picker.Title)
	m.picker.OnConfirm(m.picker.Items)
	msg := m.machinePending().(machineInstallPlanMsg)
	require.NoError(t, msg.err)
	require.Len(t, msg.plan.Directories, 20)
	require.Len(t, msg.plan.Items, 80)
	m.acceptMachineInstallPlan(msg)
	require.Contains(t, m.confirm.Prompt, "blocked 80")
	m.handleConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	msg = m.machinePending().(machineInstallPlanMsg)
	require.NoError(t, msg.err)
	m.acceptMachineInstallPlan(msg)
	require.NotNil(t, m.confirm)
	require.Contains(t, m.confirm.Prompt, "80 locations across 20 directories")
	require.Contains(t, m.confirm.Prompt, "extra, team")
	for _, project := range projects {
		require.NotContains(t, m.confirm.Prompt, project)
	}
	require.Equal(t, 80, strings.Count(strings.TrimSpace(m.confirm.Diff), "\n")+1)
	m.confirm.OnYes()
	done := m.machinePending().(machineInstallDoneMsg)
	require.NoError(t, done.err)
	require.Len(t, done.result.Items, 80)
	for _, item := range done.result.Items {
		require.Equal(t, "installed", item.Status)
	}
	for _, project := range projects {
		for _, client := range []string{".agents", ".claude"} {
			for _, skill := range []string{"skill-a", "skill-b"} {
				_, err := os.Readlink(filepath.Join(project, client, "skills", skill))
				require.NoError(t, err)
			}
		}
	}
}

func TestMachineInstallRejectsEmptyDirectorySelection(t *testing.T) {
	m := newTestModel(t)
	m.machine.open = true
	m.requestMachineInstallPlan(services.MachineBatchRequest{Skills: []string{"skill-a"}, Targets: []string{"codex"}, Tags: []string{"empty"}})
	msg := m.machinePending().(machineInstallPlanMsg)
	require.NoError(t, msg.err)
	require.Empty(t, msg.plan.Directories)
	m.acceptMachineInstallPlan(msg)
	require.Nil(t, m.confirm)
	require.ErrorContains(t, m.machine.err, "no directories match")
}

func TestMachineInstallReportsConflictAndContinues(t *testing.T) {
	m := newTestModel(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, m.svc.RegisterDirectories(t.Context(), map[string]string{"project": project}, []string{"team"}))
	occupied := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.MkdirAll(filepath.Dir(occupied), 0o755))
	require.NoError(t, os.WriteFile(occupied, []byte("outside"), 0o644))
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	m.requestMachineInstallPlan(services.MachineBatchRequest{Skills: []string{"skill-a"}, Targets: []string{"codex", "claude"}, Tags: []string{"team"}, AllowUnignored: true})
	preview := m.machinePending().(machineInstallPlanMsg)
	require.NoError(t, preview.err)
	m.acceptMachineInstallPlan(preview)
	require.Contains(t, m.confirm.Prompt, "blocked 1")
	m.confirm.OnYes()
	done := m.machinePending().(machineInstallDoneMsg)
	require.NoError(t, done.err)
	statuses := map[string]string{}
	for _, item := range done.result.Items {
		statuses[item.Target] = item.Status
	}
	require.Equal(t, "conflict", statuses["codex"])
	require.Equal(t, "installed", statuses["claude"])
	data, err := os.ReadFile(occupied)
	require.NoError(t, err)
	require.Equal(t, "outside", string(data))
	scan = m.acceptMachineInstallDone(done)
	require.NotNil(t, scan)
	m.acceptMachineScan(scan().(machineScanMsg))
	require.Contains(t, m.machine.result, "preview the remaining work")
	require.NoError(t, os.Remove(occupied))
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	retry := m.machinePending().(machineInstallPlanMsg)
	require.NoError(t, retry.err)
	statuses = map[string]string{}
	for _, item := range retry.plan.Items {
		statuses[item.Target] = item.Status
	}
	require.Equal(t, "unignored", statuses["codex"])
	require.Equal(t, "installed", statuses["claude"])
}

func TestMachineInstallCanRepreviewStalePlan(t *testing.T) {
	m := newTestModel(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, m.svc.RegisterDirectories(t.Context(), map[string]string{"project": project}, []string{"team"}))
	m.openMachineDirectories()
	scan := m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	m.acceptMachineScan(scan().(machineScanMsg))
	m.requestMachineInstallPlan(services.MachineBatchRequest{Skills: []string{"skill-a"}, Targets: []string{"codex"}, Tags: []string{"team"}, AllowUnignored: true})
	preview := m.machinePending().(machineInstallPlanMsg)
	require.NoError(t, preview.err)
	m.acceptMachineInstallPlan(preview)
	occupied := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.MkdirAll(filepath.Dir(occupied), 0o755))
	require.NoError(t, os.WriteFile(occupied, []byte("outside"), 0o644))
	m.confirm.OnYes()
	done := m.machinePending().(machineInstallDoneMsg)
	require.ErrorContains(t, done.err, "stale")
	m.acceptMachineInstallDone(done)
	require.Contains(t, m.machineView(), "preview again")
	m.handleMachineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	refreshed := m.machinePending().(machineInstallPlanMsg)
	require.NoError(t, refreshed.err)
	require.Equal(t, "conflict", refreshed.plan.Items[0].Status)
}

func TestUnifiedInstallFromSkillKeepsOriginAndLocalPreference(t *testing.T) {
	m := newTestModel(t)
	m.svc.Cfg.UserSkillInstallLocalOnly = true
	m.openDetail()
	m.handleKey(runeKey('i'))
	m.acceptMachineDirectories(m.machinePending().(machineDirectoriesMsg))
	require.Equal(t, "installation scope", m.picker.Title)
	m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter}) // User
	require.Equal(t, "select clients", m.picker.Title)
	for i := range m.picker.Items {
		m.picker.Items[i].Checked = m.picker.Items[i].Value == "t"
	}
	m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	msg := m.machinePending().(machineInstallPlanMsg)
	require.NoError(t, msg.err)
	require.True(t, msg.plan.Request.LocalOnly)
	m.acceptMachineInstallPlan(msg)
	require.Contains(t, m.confirm.Prompt, "Local only")
	m.handleConfirmKey(runeKey('w'))
	msg = m.machinePending().(machineInstallPlanMsg)
	require.NoError(t, msg.err)
	require.False(t, msg.plan.Request.LocalOnly)
	m.acceptMachineInstallPlan(msg)
	require.Contains(t, m.confirm.Prompt, "New installation declarations: 1")
	m.handleConfirmKey(runeKey('w'))
	msg = m.machinePending().(machineInstallPlanMsg)
	require.NoError(t, msg.err)
	m.acceptMachineInstallPlan(msg)
	m.handleConfirmKey(runeKey('y'))
	done := m.machinePending().(machineInstallDoneMsg)
	require.NoError(t, done.err)
	m.acceptMachineInstallDone(done)
	require.True(t, m.showDetail)
	require.False(t, m.machine.open)
	target, ok := m.svc.Installer.TargetByName("t")
	require.True(t, ok)
	require.FileExists(t, filepath.Join(target.Path, "skill-a", "SKILL.md"))
	declarations, err := m.svc.ListMachineDeclarations()
	require.NoError(t, err)
	require.Empty(t, declarations)
}
