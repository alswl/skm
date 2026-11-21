package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestMachineBatchPlanAndApply(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: review\n---\n"), 0o644))
	svc := newUpdateSvc(t, root)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, []string{"team", "extra"}))
	request := MachineBatchRequest{Skills: []string{"review"}, Targets: []string{"codex", "claude"}, Tags: []string{"team", "extra"}, AllowUnignored: true}
	plan, err := svc.PlanMachineBatch(request)
	require.NoError(t, err)
	require.Len(t, plan.Items, 2)
	require.Len(t, plan.Declarations, 4)
	// A changed selected slot invalidates the whole preview before declarations change.
	slot := filepath.Join(project, ".agents", "skills", "review")
	require.NoError(t, os.MkdirAll(filepath.Dir(slot), 0o755))
	require.NoError(t, os.WriteFile(slot, []byte("external"), 0o644))
	_, err = svc.ApplyMachineBatch(context.Background(), *plan)
	require.Error(t, err)
	doc, err := config.LoadMachineDocument(filepath.Join(root, "machine.yaml"))
	require.NoError(t, err)
	require.Empty(t, doc.Installs)
	plan, err = svc.PlanMachineBatch(request)
	require.NoError(t, err)
	result, err := svc.ApplyMachineBatch(context.Background(), *plan)
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	statuses := map[string]string{}
	for _, item := range result.Items {
		statuses[item.Target] = item.Status
	}
	require.Equal(t, "conflict", statuses["codex"])
	require.Equal(t, "installed", statuses["claude"])
	data, err := os.ReadFile(slot)
	require.NoError(t, err)
	require.Equal(t, "external", string(data))
	doc, err = config.LoadMachineDocument(filepath.Join(root, "machine.yaml"))
	require.NoError(t, err)
	require.Len(t, doc.Installs, 4)
}

func TestMachineBatchUserAndDirectoryScopes(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: review\n---\n"), 0o644))
	userDir := filepath.Join(t.TempDir(), "new", "skills")
	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{skillTarget("user-client", userDir)}}, common.NewLogger(false))
	require.NoError(t, err)
	userPlan, err := svc.PlanMachineBatch(MachineBatchRequest{Skills: []string{"review"}, Targets: []string{"user-client"}, User: true})
	require.NoError(t, err)
	require.Len(t, userPlan.Items, 1)
	require.Equal(t, "install", userPlan.Items[0].Status)
	result, err := svc.ApplyMachineBatch(context.Background(), *userPlan)
	require.NoError(t, err)
	require.Equal(t, "installed", result.Items[0].Status)
	got, err := os.Readlink(filepath.Join(userDir, "review"))
	require.NoError(t, err)
	require.Equal(t, skill, got)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, nil))
	dirPlan, err := svc.PlanMachineBatch(MachineBatchRequest{Skills: []string{"review"}, Targets: []string{"codex"}, Directories: []string{"project"}, AllowUnignored: true})
	require.NoError(t, err)
	_, err = svc.ApplyMachineBatch(context.Background(), *dirPlan)
	require.NoError(t, err)
	got, err = os.Readlink(filepath.Join(project, ".agents", "skills", "review"))
	require.NoError(t, err)
	require.Equal(t, skill, got)
	doc, err := config.LoadMachineDocument(filepath.Join(root, "machine.yaml"))
	require.NoError(t, err)
	require.Len(t, doc.Installs, 2)
	require.Equal(t, "user", doc.Installs[0].Scope.Kind)
	require.Equal(t, "directory", doc.Installs[1].Scope.Kind)
}

func TestMachineBatchConflictingSkillsDoNotBlockIndependentItems(t *testing.T) {
	root := t.TempDir()
	for _, ref := range []string{"skills/local/review", "skills/other/review", "skills/local/check"} {
		dir := filepath.Join(root, ref)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+filepath.Base(ref)+"\ndescription: test\n---\n"), 0o644))
	}
	svc := newUpdateSvc(t, root)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, []string{"team"}))
	plan, err := svc.PlanMachineBatch(MachineBatchRequest{Skills: []string{"skills/local/review", "skills/other/review", "skills/local/check"}, Targets: []string{"codex"}, Tags: []string{"team"}, AllowUnignored: true})
	require.NoError(t, err)
	require.Len(t, plan.Items, 3)
	result, err := svc.ApplyMachineBatch(context.Background(), *plan)
	require.NoError(t, err)
	statuses := map[string]string{}
	for _, item := range result.Items {
		statuses[item.Skill] = item.Status
	}
	require.Equal(t, "conflict", statuses["skills/local/review"])
	require.Equal(t, "conflict", statuses["skills/other/review"])
	require.Equal(t, "installed", statuses["skills/local/check"])
	_, err = os.Lstat(filepath.Join(project, ".agents", "skills", "review"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Readlink(filepath.Join(project, ".agents", "skills", "check"))
	require.NoError(t, err)
}

func TestMachineBatchRechecksExistingCoverage(t *testing.T) {
	root := t.TempDir()
	for _, ref := range []string{"skills/local/review", "skills/other/review", "skills/local/check"} {
		dir := filepath.Join(root, ref)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+filepath.Base(ref)+"\ndescription: test\n---\n"), 0o644))
	}
	svc := newUpdateSvc(t, root)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, []string{"team"}))
	request := MachineBatchRequest{Skills: []string{"skills/local/review", "skills/local/check"}, Targets: []string{"codex"}, Directories: []string{"project"}, AllowUnignored: true}
	plan, err := svc.PlanMachineBatch(request)
	require.NoError(t, err)
	manifest := filepath.Join(root, "machine.yaml")
	doc, err := config.LoadMachineDocument(manifest)
	require.NoError(t, err)
	doc.Installs = append(doc.Installs, common.MachineInstall{Skill: "skills/other/review", Target: "codex", Scope: common.MachineScope{Kind: "tag", Name: "team"}})
	require.NoError(t, config.WriteMachineYAML(manifest, doc, 0o644))
	_, err = svc.ApplyMachineBatch(context.Background(), *plan)
	require.ErrorContains(t, err, "stale")
	plan, err = svc.PlanMachineBatch(request)
	require.NoError(t, err)
	result, err := svc.ApplyMachineBatch(context.Background(), *plan)
	require.NoError(t, err)
	for _, item := range result.Items {
		if item.Skill == "skills/local/review" {
			require.Equal(t, "conflict", item.Status)
		} else {
			require.Equal(t, "installed", item.Status)
		}
	}
}

func TestMachineBatchDetectsCoverageThroughParentAlias(t *testing.T) {
	root := t.TempDir()
	for _, ref := range []string{"skills/local/review", "skills/other/review"} {
		dir := filepath.Join(root, ref)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review\ndescription: test\n---\n"), 0o644))
	}
	userDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{skillTarget("client", userDir)}}, common.NewLogger(false))
	require.NoError(t, err)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".agents"), 0o755))
	require.NoError(t, os.Symlink(userDir, filepath.Join(project, ".agents", "skills")))
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, nil))
	manifest := filepath.Join(root, "machine.yaml")
	doc, err := config.LoadMachineDocument(manifest)
	require.NoError(t, err)
	doc.Installs = []common.MachineInstall{{Skill: "skills/other/review", Target: "codex", Scope: common.MachineScope{Kind: "directory", Name: "project"}}}
	require.NoError(t, config.WriteMachineYAML(manifest, doc, 0o644))
	plan, err := svc.PlanMachineBatch(MachineBatchRequest{Skills: []string{"skills/local/review"}, Targets: []string{"client"}, User: true})
	require.NoError(t, err)
	require.Len(t, plan.Items, 1)
	require.Equal(t, "conflict", plan.Items[0].Status)
}

func TestMachineBatchRechecksSourceDuringExecution(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(root, "skills", "local", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\n"), 0o644))
	}
	svc := newUpdateSvc(t, root)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, []string{"team"}))
	plan, err := svc.PlanMachineBatch(MachineBatchRequest{Skills: []string{"alpha", "beta"}, Targets: []string{"codex"}, Tags: []string{"team"}, AllowUnignored: true})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "skills", "local", "alpha", "SKILL.md"), []byte("---\nname: alpha\ndescription: changed\n---\n"), 0o644))
	result := svc.executeMachineBatchItems(plan, nil)
	status := map[string]string{}
	for _, item := range result.Items {
		status[item.Skill] = item.Status
	}
	require.Equal(t, "error", status["skills/local/alpha"])
	require.Equal(t, "installed", status["skills/local/beta"])
	_, err = os.Lstat(filepath.Join(project, ".agents", "skills", "alpha"))
	require.True(t, os.IsNotExist(err))
}
