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

func TestMachineRestoreThreeScopes(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: review\n---\n"), 0o644))
	userDir := filepath.Join(t.TempDir(), "user", "skills")
	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{skillTarget("user-client", userDir)}}, common.NewLogger(false))
	require.NoError(t, err)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, []string{"team"}))
	doc, err := config.LoadInstallations(filepath.Join(root, "installations.yaml"))
	require.NoError(t, err)
	ref := "skills/local/review"
	doc.Installs = []common.Installation{
		{Skill: ref, Target: "user-client", Scope: common.InstallationScope{Kind: "user"}},
		{Skill: ref, Target: "codex", Scope: common.InstallationScope{Kind: "directory", Name: "project"}},
		{Skill: ref, Target: "codex", Scope: common.InstallationScope{Kind: "tag", Name: "team"}},
	}
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	plan, err := svc.PlanMachineRestore(true)
	require.NoError(t, err)
	// Reordering equivalent declarations must not stale a reviewed plan.
	doc.Installs[0], doc.Installs[2] = doc.Installs[2], doc.Installs[0]
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	reordered, err := svc.PlanMachineRestore(true)
	require.NoError(t, err)
	require.Equal(t, plan.Digest, reordered.Digest)
	doc.Installs[0], doc.Installs[2] = doc.Installs[2], doc.Installs[0]
	// A declaration edit invalidates the complete preview before any link is created.
	doc.Installs = doc.Installs[:2]
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	_, err = svc.ApplyMachineRestore(context.Background(), *plan)
	require.Error(t, err)
	_, err = os.Lstat(filepath.Join(userDir, "review"))
	require.True(t, os.IsNotExist(err))
	doc.Installs = append(doc.Installs, common.Installation{Skill: ref, Target: "codex", Scope: common.InstallationScope{Kind: "tag", Name: "team"}})
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	plan, err = svc.PlanMachineRestore(true)
	require.NoError(t, err)
	result, err := svc.ApplyMachineRestore(context.Background(), *plan)
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	for _, item := range result.Items {
		require.Equal(t, "installed", item.Status)
	}
	plan, err = svc.PlanMachineRestore(true)
	require.NoError(t, err)
	result, err = svc.ApplyMachineRestore(context.Background(), *plan)
	require.NoError(t, err)
	for _, item := range result.Items {
		require.Equal(t, "installed", item.Status)
	}
	require.NoError(t, os.Remove(filepath.Join(userDir, "review")))
	plan, err = svc.PlanMachineRestore(true)
	require.NoError(t, err)
	foundMissing := false
	for _, batch := range plan.Plans {
		for _, item := range batch.Items {
			if item.Target == "user-client" && item.Status == "install" {
				foundMissing = true
			}
		}
	}
	require.True(t, foundMissing)
}

func TestMachineRestoreScopeUsesCoveredDirectories(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: review\n---\n"), 0o644))
	userDir := filepath.Join(t.TempDir(), "user")
	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{skillTarget("user-client", userDir)}}, common.NewLogger(false))
	require.NoError(t, err)
	for _, id := range []string{"a", "b"} {
		project := filepath.Join(t.TempDir(), id)
		output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
		require.NoError(t, err, string(output))
		require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{id: project}, []string{"team"}))
	}
	doc, err := config.LoadInstallations(filepath.Join(root, "installations.yaml"))
	require.NoError(t, err)
	doc.Installs = []common.Installation{
		{Skill: "skills/local/review", Target: "codex", Scope: common.InstallationScope{Kind: "tag", Name: "team"}},
		{Skill: "skills/local/review", Target: "user-client", Scope: common.InstallationScope{Kind: "user"}},
	}
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	plan, err := svc.PlanMachineRestoreScoped(MachineRestoreRequest{Directories: []string{"a"}, AllowUnignored: true})
	require.NoError(t, err)
	require.Len(t, plan.Plans, 1)
	require.Len(t, plan.Plans[0].Items, 1)
	require.Equal(t, "a", plan.Plans[0].Items[0].Directory)
	// Changes outside the selected directory and unrelated tags do not stale it.
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "review"), []byte("outside"), 0o644))
	require.NoError(t, svc.TagDirectories(context.Background(), []string{"a"}, []string{"unrelated"}, nil))
	result, err := svc.ApplyMachineRestore(context.Background(), *plan)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "installed", result.Items[0].Status)
	userPlan, err := svc.PlanMachineRestoreScoped(MachineRestoreRequest{User: true})
	require.NoError(t, err)
	require.Len(t, userPlan.Plans, 1)
	require.Equal(t, "conflict", userPlan.Plans[0].Items[0].Status)
	_, err = svc.PlanMachineRestoreScoped(MachineRestoreRequest{User: true, Tags: []string{"team"}})
	require.Error(t, err)
}

func TestMachineRestoreAcrossLibraryCheckouts(t *testing.T) {
	configDir := t.TempDir()
	firstRoot, secondRoot := t.TempDir(), t.TempDir()
	for _, root := range []string{firstRoot, secondRoot} {
		skill := filepath.Join(root, "skills", "local", "review")
		require.NoError(t, os.MkdirAll(skill, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: review\n---\n"), 0o644))
	}
	firstUser, secondUser := filepath.Join(t.TempDir(), "user"), filepath.Join(t.TempDir(), "user")
	first, err := New(&config.Config{Root: firstRoot, ConfigDir: configDir, Targets: []common.InstallTarget{skillTarget("client", firstUser)}}, common.NewLogger(false))
	require.NoError(t, err)
	second, err := New(&config.Config{Root: secondRoot, ConfigDir: configDir, Targets: []common.InstallTarget{skillTarget("client", secondUser)}}, common.NewLogger(false))
	require.NoError(t, err)
	projects := []string{filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")}
	for _, project := range projects {
		output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
		require.NoError(t, err, string(output))
	}
	require.NoError(t, first.RegisterDirectories(context.Background(), map[string]string{"project": projects[0]}, []string{"team"}))
	doc, err := config.LoadInstallations(filepath.Join(firstRoot, "installations.yaml"))
	require.NoError(t, err)
	ref := "skills/local/review"
	doc.Installs = []common.Installation{
		{Skill: ref, Target: "client", Scope: common.InstallationScope{Kind: "user"}},
		{Skill: ref, Target: "codex", Scope: common.InstallationScope{Kind: "directory", Name: "project"}},
		{Skill: ref, Target: "claude", Scope: common.InstallationScope{Kind: "tag", Name: "team"}},
	}
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(firstRoot, "installations.yaml"), doc, 0o644))
	data, err := os.ReadFile(filepath.Join(firstRoot, "installations.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(secondRoot, "installations.yaml"), data, 0o644))
	before, err := second.PlanMachineRestore(true)
	require.NoError(t, err)
	missingPaths := 0
	for _, batch := range before.Plans {
		for _, item := range batch.Items {
			if item.Status == "missing_path" {
				missingPaths++
			}
		}
	}
	require.Equal(t, 2, missingPaths)
	require.NoError(t, second.SetDirectoryPath(context.Background(), "project", projects[1]))
	plan, err := second.PlanMachineRestore(true)
	require.NoError(t, err)
	result, err := second.ApplyMachineRestore(context.Background(), *plan)
	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	for _, item := range result.Items {
		require.Equal(t, "installed", item.Status)
	}
	for _, link := range []string{filepath.Join(secondUser, "review"), filepath.Join(projects[1], ".agents", "skills", "review"), filepath.Join(projects[1], ".claude", "skills", "review")} {
		target, err := os.Readlink(link)
		require.NoError(t, err)
		require.Equal(t, filepath.Join(secondRoot, ref), target)
	}
	again, err := second.PlanMachineRestore(true)
	require.NoError(t, err)
	for _, batch := range again.Plans {
		for _, item := range batch.Items {
			require.Equal(t, "installed", item.Status)
		}
	}
	_, err = os.Lstat(filepath.Join(firstUser, "review"))
	require.True(t, os.IsNotExist(err))
}

func TestMachineRestoreReportsUnavailableSkillAndContinues(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: review\n---\n"), 0o644))
	userDir := filepath.Join(t.TempDir(), "skills")
	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{skillTarget("client", userDir)}}, common.NewLogger(false))
	require.NoError(t, err)
	doc := config.InstallationsDocument{Version: 1, Installs: []common.Installation{
		{Skill: "skills/local/review", Target: "client", Scope: common.InstallationScope{Kind: "user"}},
		{Skill: "skills/local/missing", Target: "client", Scope: common.InstallationScope{Kind: "user"}},
	}}
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	plan, err := svc.PlanMachineRestore(false)
	require.NoError(t, err)
	require.Len(t, plan.Plans, 2)
	statuses := map[string]string{}
	for _, batch := range plan.Plans {
		for _, item := range batch.Items {
			statuses[item.Skill] = item.Status
		}
	}
	require.Equal(t, "error", statuses["skills/local/missing"])
	result, err := svc.ApplyMachineRestore(context.Background(), *plan)
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	statuses = map[string]string{}
	for _, item := range result.Items {
		statuses[item.Skill] = item.Status
	}
	require.Equal(t, "installed", statuses["skills/local/review"])
	require.Equal(t, "error", statuses["skills/local/missing"])
	_, err = os.Readlink(filepath.Join(userDir, "review"))
	require.NoError(t, err)
}

func TestMachineRestoreReportsUndeclaredWithoutExtendingApply(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"review", "manual"} {
		dir := filepath.Join(root, "skills", "local", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\n"), 0o644))
	}
	userDir := filepath.Join(t.TempDir(), "skills")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	manual := filepath.Join(userDir, "manual")
	require.NoError(t, os.Symlink(filepath.Join(root, "skills", "local", "manual"), manual))
	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{skillTarget("client", userDir)}}, common.NewLogger(false))
	require.NoError(t, err)
	doc := config.InstallationsDocument{Version: 1, Installs: []common.Installation{{Skill: "skills/local/review", Target: "client", Scope: common.InstallationScope{Kind: "user"}}}}
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	plan, err := svc.PlanMachineRestore(false)
	require.NoError(t, err)
	require.Len(t, plan.Observations, 1)
	require.Equal(t, "skills/local/manual", plan.Observations[0].Skill)
	require.False(t, plan.Observations[0].Declared)
	require.NoError(t, os.Remove(manual))
	result, err := svc.ApplyMachineRestore(context.Background(), *plan)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "installed", result.Items[0].Status)
	_, err = os.Lstat(manual)
	require.True(t, os.IsNotExist(err))
}
