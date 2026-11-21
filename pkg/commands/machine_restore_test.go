package commands

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestMachineDropMissingSourceByDeclaration(t *testing.T) {
	root, cfg := cmdFixture(t)
	ref := "skills/local/skill-a"
	source := filepath.Join(root, ref)
	link := filepath.Join(t.TempDir(), "skill-a")
	require.NoError(t, os.Symlink(source, link))
	doc := config.MachineDocument{Version: 1, Installs: []common.MachineInstall{{Skill: ref, Target: "t", Scope: common.MachineScope{Kind: "user"}}}}
	require.NoError(t, config.WriteMachineYAML(filepath.Join(root, "machine.yaml"), doc, 0o644))
	require.NoError(t, os.RemoveAll(source))
	plan, err := runCmd(t, "machine", "plan", "--drop", ref, "--target", "t", "--user", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var exported machinePlanDocument
	require.NoError(t, json.Unmarshal([]byte(plan), &exported))
	require.Equal(t, "drop", exported.Action)
	file := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(file, []byte(plan), 0o600))
	_, err = runCmd(t, "machine", "apply", "--plan", file, "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	loaded, err := config.LoadMachineDocument(filepath.Join(root, "machine.yaml"))
	require.NoError(t, err)
	require.Empty(t, loaded.Installs)
	_, err = os.Readlink(link)
	require.NoError(t, err)
}

func TestMachineRestoreExportRejectsStaleTarget(t *testing.T) {
	root, cfg := cmdFixture(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	_, err = runCmd(t, "machine", "directory", "add", "project="+project, "--tag", "team", "--root", root, "--config", cfg)
	require.NoError(t, err)
	doc, err := config.LoadMachineDocument(filepath.Join(root, "machine.yaml"))
	require.NoError(t, err)
	doc.Installs = []common.MachineInstall{{Skill: "skills/local/skill-a", Target: "codex", Scope: common.MachineScope{Kind: "tag", Name: "team"}}}
	require.NoError(t, config.WriteMachineYAML(filepath.Join(root, "machine.yaml"), doc, 0o644))
	plan, err := runCmd(t, "machine", "plan", "--allow-unignored", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "restore.json")
	require.NoError(t, os.WriteFile(file, []byte(plan), 0o600))
	occupied := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.MkdirAll(filepath.Dir(occupied), 0o755))
	require.NoError(t, os.WriteFile(occupied, []byte("outside"), 0o644))
	_, err = runCmd(t, "machine", "apply", "--plan", file, "--json", "--root", root, "--config", cfg)
	require.Error(t, err)
	require.Equal(t, common.ExitObject, common.ExitCodeOf(err, 0))
	data, err := os.ReadFile(occupied)
	require.NoError(t, err)
	require.Equal(t, "outside", string(data))
}

func TestMachineUnlinkCoveredPreviewCannotBeAppliedAfterDrop(t *testing.T) {
	root, cfg := cmdFixture(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	_, err = runCmd(t, "machine", "directory", "add", "project="+project, "--root", root, "--config", cfg)
	require.NoError(t, err)
	doc, err := config.LoadMachineDocument(filepath.Join(root, "machine.yaml"))
	require.NoError(t, err)
	doc.Installs = []common.MachineInstall{{Skill: "skills/local/skill-a", Target: "codex", Scope: common.MachineScope{Kind: "directory", Name: "project"}}}
	require.NoError(t, config.WriteMachineYAML(filepath.Join(root, "machine.yaml"), doc, 0o644))
	link := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(root, "skills/local/skill-a"), link))
	plan, err := runCmd(t, "machine", "plan", "--unlink", link, "--target", "codex", "--directory", "project", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var exported machinePlanDocument
	require.NoError(t, json.Unmarshal([]byte(plan), &exported))
	require.Len(t, exported.Diagnostics, 1)
	require.Contains(t, exported.Diagnostics[0].Error, "covered")
	file := filepath.Join(t.TempDir(), "blocked.json")
	require.NoError(t, os.WriteFile(file, []byte(plan), 0o600))
	doc.Installs = nil
	require.NoError(t, config.WriteMachineYAML(filepath.Join(root, "machine.yaml"), doc, 0o644))
	_, err = runCmd(t, "machine", "apply", "--plan", file, "--json", "--root", root, "--config", cfg)
	require.Error(t, err)
	require.Equal(t, common.ExitObject, common.ExitCodeOf(err, 0))
	_, err = os.Readlink(link)
	require.NoError(t, err)
}

func TestMachineUnlinkPreviewReportsUnknownCoverage(t *testing.T) {
	root, cfg := cmdFixture(t)
	project := filepath.Join(t.TempDir(), "project")
	other := filepath.Join(t.TempDir(), "other")
	for _, dir := range []string{project, other} {
		output, err := exec.Command("git", "init", "-q", dir).CombinedOutput()
		require.NoError(t, err, string(output))
	}
	_, err := runCmd(t, "machine", "directory", "add", "project="+project, "other="+other, "--root", root, "--config", cfg)
	require.NoError(t, err)
	doc, err := config.LoadMachineDocument(filepath.Join(root, "machine.yaml"))
	require.NoError(t, err)
	doc.Installs = []common.MachineInstall{{Skill: "skills/local/skill-a", Target: "codex", Scope: common.MachineScope{Kind: "directory", Name: "other"}}}
	require.NoError(t, config.WriteMachineYAML(filepath.Join(root, "machine.yaml"), doc, 0o644))
	link := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(root, "skills/local/skill-a"), link))
	require.NoError(t, os.RemoveAll(other))
	output, err := runCmd(t, "machine", "plan", "--unlink", link, "--target", "codex", "--directory", "project", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var plan machinePlanDocument
	require.NoError(t, json.Unmarshal([]byte(output), &plan))
	require.Len(t, plan.Diagnostics, 1)
	require.Contains(t, plan.Diagnostics[0].Error, "cannot establish declaration coverage")
	_, err = os.Readlink(link)
	require.NoError(t, err)
}
