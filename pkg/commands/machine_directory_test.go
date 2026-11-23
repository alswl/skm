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

func TestMachineDirectoryScanCaptureCLI(t *testing.T) {
	root, cfg := cmdFixture(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	_, err = runCmd(t, "directory", "add", "project="+project, "--tag", "team", "--root", root, "--config", cfg)
	require.NoError(t, err)
	target := filepath.Join(project, ".agents", "skills")
	require.NoError(t, os.MkdirAll(target, 0o755))
	link := filepath.Join(target, "skill-a")
	require.NoError(t, os.Symlink(filepath.Join(root, "skills", "local", "skill-a"), link))

	scanned, err := runCmd(t, "scan", "--directory", "project", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var report struct {
		Items []struct {
			Skill string `json:"skill"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(scanned), &report))
	require.Len(t, report.Items, 1)
	require.Equal(t, "skills/local/skill-a", report.Items[0].Skill)

	plan, err := runCmd(t, "plan", "--capture", link, "--target", "codex", "--directory", "project", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planPath, []byte(plan), 0o600))
	_, err = runCmd(t, "apply", "--plan", planPath, "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	doc, err := config.LoadInstallations(filepath.Join(root, "installations.yaml"))
	require.NoError(t, err)
	require.Len(t, doc.Installs, 1)
	require.Equal(t, "project", doc.Installs[0].Scope.Name)
	require.FileExists(t, link)
	planCmd, _, err := rootCmd.Find([]string{"plan"})
	require.NoError(t, err)
	require.NoError(t, planCmd.Flags().Set("capture", ""))
	drop, err := runCmd(t, "plan", "--drop", "skill-a", "--target", "codex", "--directory", "project", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(planPath, []byte(drop), 0o600))
	_, err = runCmd(t, "apply", "--plan", planPath, "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	doc, err = config.LoadInstallations(filepath.Join(root, "installations.yaml"))
	require.NoError(t, err)
	require.Empty(t, doc.Installs)
	require.FileExists(t, link)
	unlink, err := runCmd(t, "plan", "--unlink", link, "--target", "codex", "--directory", "project", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(planPath, []byte(unlink), 0o600))
	_, err = runCmd(t, "apply", "--plan", planPath, "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err))
}

func TestMachineCaptureUserCLI(t *testing.T) {
	root, cfg := cmdFixture(t)
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(target, 0o755))
	writeTestFile(t, cfg, "config.yaml", "targets:\n  - name: t\n    path: "+target+"\n    accepts: [skill]\n    strategies:\n      skill: skill-symlink\n")
	link := filepath.Join(target, "skill-a")
	require.NoError(t, os.Symlink(filepath.Join(root, "skills", "local", "skill-a"), link))
	planCmd, _, err := rootCmd.Find([]string{"plan"})
	require.NoError(t, err)
	for _, flag := range []string{"directory", "drop", "tag"} {
		require.NoError(t, planCmd.Flags().Set(flag, ""))
	}
	plan, err := runCmd(t, "plan", "--capture", link, "--target", "t", "--user", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planPath, []byte(plan), 0o600))
	_, err = runCmd(t, "apply", "--plan", planPath, "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	doc, err := config.LoadInstallations(filepath.Join(root, "installations.yaml"))
	require.NoError(t, err)
	require.Len(t, doc.Installs, 1)
	require.Equal(t, "user", doc.Installs[0].Scope.Kind)
	require.FileExists(t, link)
	require.NoError(t, planCmd.Flags().Set("user", "false"))
	require.NoError(t, planCmd.Flags().Set("capture", ""))
}

func TestMachineBatchCLI(t *testing.T) {
	root, cfg := cmdFixture(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	_, err = runCmd(t, "directory", "add", "project="+project, "--tag", "team", "--root", root, "--config", cfg)
	require.NoError(t, err)
	args := []string{"plan", "--add", "skill-a", "--target", "codex", "--target", "claude", "--tag", "team", "--allow-unignored", "--root", root, "--config", cfg}
	summary, err := runCmd(t, args...)
	require.NoError(t, err)
	require.Contains(t, summary, "2 installation locations")
	require.NotContains(t, summary, project)
	plan, err := runCmd(t, append(args, "--json")...)
	require.NoError(t, err)
	var exported map[string]any
	require.NoError(t, json.Unmarshal([]byte(plan), &exported))
	for _, key := range []string{"version", "request", "digest", "items", "diagnostics", "summary"} {
		require.Contains(t, exported, key)
	}
	// Display items are not write instructions. Changing one must not redirect Apply.
	items := exported["items"].([]any)
	items[0].(map[string]any)["path"] = filepath.Join(t.TempDir(), "wrong")
	modified, err := json.Marshal(exported)
	require.NoError(t, err)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planPath, modified, 0o600))
	_, err = runCmd(t, "apply", "--plan", planPath, "--dry-run", "--root", root, "--config", cfg)
	require.Error(t, err)
	_, err = os.Lstat(filepath.Join(project, ".agents", "skills", "skill-a"))
	require.True(t, os.IsNotExist(err))
	_, err = runCmd(t, "apply", "--plan", planPath, "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	for _, dir := range []string{".agents", ".claude"} {
		link := filepath.Join(project, dir, "skills", "skill-a")
		got, err := os.Readlink(link)
		require.NoError(t, err)
		require.Equal(t, filepath.Join(root, "skills", "local", "skill-a"), got)
	}
	missing := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.Remove(missing))
	restore, err := runCmd(t, "plan", "--allow-unignored", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(planPath, []byte(restore), 0o600))
	_, err = runCmd(t, "apply", "--plan", planPath, "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	_, err = os.Readlink(missing)
	require.NoError(t, err)
}

func TestMachinePlanRejectsBadFormatAndStalePlan(t *testing.T) {
	root, cfg := cmdFixture(t)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planPath, []byte(`{"version":1,"action":"add","request":{},"digest":"bad"}`), 0o600))
	_, err := runCmd(t, "apply", "--plan", planPath, "--json", "--root", root, "--config", cfg)
	require.Error(t, err)
	require.Equal(t, common.ExitError, common.ExitCodeOf(err, 0))
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	_, err = runCmd(t, "directory", "add", "project="+project, "--tag", "team", "--root", root, "--config", cfg)
	require.NoError(t, err)
	plan, err := runCmd(t, "plan", "--add", "skill-a", "--target", "codex", "--tag", "team", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(planPath, []byte(plan), 0o600))
	link := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.WriteFile(link, []byte("outside"), 0o644))
	_, err = runCmd(t, "apply", "--plan", planPath, "--json", "--root", root, "--config", cfg)
	require.Error(t, err)
	require.Equal(t, common.ExitObject, common.ExitCodeOf(err, 0))
}

func TestMachineCaptureMultipleIndependentLinks(t *testing.T) {
	root, cfg := cmdFixture(t)
	writeTestFile(t, root, "skills/local/skill-b/SKILL.md", "---\nname: skill-b\ndescription: B\n---\n")
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	_, err = runCmd(t, "directory", "add", "project="+project, "--root", root, "--config", cfg)
	require.NoError(t, err)
	_, err = runCmd(t, "plan", "--capture", filepath.Join(project, "missing"), "--target", "unknown", "--directory", "project", "--json", "--root", root, "--config", cfg)
	require.Error(t, err)
	dir := filepath.Join(project, ".agents", "skills")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, name := range []string{"skill-a", "skill-b"} {
		require.NoError(t, os.Symlink(filepath.Join(root, "skills", "local", name), filepath.Join(dir, name)))
	}
	external := filepath.Join(dir, "external")
	require.NoError(t, os.WriteFile(external, []byte("outside"), 0o644))
	plan, err := runCmd(t, "plan", "--capture", filepath.Join(dir, "skill-a"), "--capture", filepath.Join(dir, "skill-b"), "--capture", external, "--target", "codex", "--directory", "project", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var preview struct {
		Diagnostics []any `json:"diagnostics"`
	}
	require.NoError(t, json.Unmarshal([]byte(plan), &preview))
	require.Len(t, preview.Diagnostics, 1)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planPath, []byte(plan), 0o600))
	result, err := runCmd(t, "apply", "--plan", planPath, "--json", "--root", root, "--config", cfg)
	require.Error(t, err)
	require.Equal(t, common.ExitObject, common.ExitCodeOf(err, 0))
	require.Contains(t, result, `"status":"captured"`)
	doc, err := config.LoadInstallations(filepath.Join(root, "installations.yaml"))
	require.NoError(t, err)
	require.Len(t, doc.Installs, 2)
	data, err := os.ReadFile(external)
	require.NoError(t, err)
	require.Equal(t, "outside", string(data))
}

func TestMachineDirectoryDiscoveryReportsRegisteredAndInvalid(t *testing.T) {
	root, cfg := cmdFixture(t)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	project := filepath.Join(parent, "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	invalid := filepath.Join(parent, "invalid")
	require.NoError(t, os.Mkdir(invalid, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(invalid, ".git"), []byte("bad"), 0o644))
	_, err = runCmd(t, "directory", "add", "project="+project, "--root", root, "--config", cfg)
	require.NoError(t, err)
	response, err := runCmd(t, "directory", "discover", parent, "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var report []struct {
		Path       string `json:"path"`
		Directory  string `json:"directory"`
		Registered bool   `json:"registered"`
		Error      string `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(response), &report))
	require.Len(t, report, 2)
	require.Equal(t, invalid, report[0].Path)
	require.NotEmpty(t, report[0].Error)
	require.Equal(t, "project", report[1].Directory)
	require.True(t, report[1].Registered)
}

func TestMachineScanAndPlanShowDesiredAndUndeclared(t *testing.T) {
	root, cfg := cmdFixture(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	_, err = runCmd(t, "directory", "add", "project="+project, "--root", root, "--config", cfg)
	require.NoError(t, err)
	link := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(root, "skills", "local", "skill-a"), link))
	plan, err := runCmd(t, "plan", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var preview struct {
		Items []struct {
			Status string `json:"status"`
			Path   string `json:"path"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(plan), &preview))
	require.Len(t, preview.Items, 1)
	require.Equal(t, "undeclared", preview.Items[0].Status)
	capture, err := runCmd(t, "plan", "--capture", link, "--target", "codex", "--directory", "project", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(file, []byte(capture), 0o600))
	_, err = runCmd(t, "apply", "--plan", file, "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	require.NoError(t, os.Remove(link))
	scan, err := runCmd(t, "scan", "--directory", "project", "--skill", "skill-a", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var report struct {
		Items []struct {
			Status   string `json:"status"`
			Declared bool   `json:"declared"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(scan), &report))
	require.Len(t, report.Items, 1)
	require.Equal(t, "missing", report.Items[0].Status)
	require.True(t, report.Items[0].Declared)
}

func TestMachineBatchPlanReportsConflictAndContinues(t *testing.T) {
	root, cfg := cmdFixture(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	_, err = runCmd(t, "directory", "add", "project="+project, "--tag", "team", "--root", root, "--config", cfg)
	require.NoError(t, err)
	zero, err := runCmd(t, "plan", "--add", "skill-a", "--target", "codex", "--tag", "empty", "--root", root, "--config", cfg)
	require.NoError(t, err)
	require.Contains(t, zero, "0 directories, 0 installation locations")
	occupied := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.MkdirAll(filepath.Dir(occupied), 0o755))
	require.NoError(t, os.WriteFile(occupied, []byte("outside"), 0o644))
	plan, err := runCmd(t, "plan", "--add", "skill-a", "--target", "codex", "--target", "claude", "--tag", "team", "--allow-unignored", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var preview struct {
		Summary map[string]int `json:"summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(plan), &preview))
	require.Equal(t, 1, preview.Summary["conflict"])
	require.Equal(t, 1, preview.Summary["unignored"])
	planPath := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(planPath, []byte(plan), 0o600))
	_, err = runCmd(t, "apply", "--plan", planPath, "--json", "--root", root, "--config", cfg)
	require.Error(t, err)
	require.Equal(t, common.ExitObject, common.ExitCodeOf(err, 0))
	data, err := os.ReadFile(occupied)
	require.NoError(t, err)
	require.Equal(t, "outside", string(data))
	_, err = os.Readlink(filepath.Join(project, ".claude", "skills", "skill-a"))
	require.NoError(t, err)
}
