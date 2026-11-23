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

func TestMachineUnlinkRequiresUncoveredUnchangedLink(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: review\n---\n"), 0o644))
	svc := newUpdateSvc(t, root)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, []string{"team"}))
	link := filepath.Join(project, ".agents", "skills", "review")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(skill, link))
	capture, err := svc.PlanMachineCapture("project", "codex", link)
	require.NoError(t, err)
	require.NoError(t, svc.ApplyMachineCapture(context.Background(), *capture))
	_, err = svc.PlanMachineUnlink("project", "codex", link)
	require.ErrorContains(t, err, "covered")
	doc, err := config.LoadInstallations(filepath.Join(root, "installations.yaml"))
	require.NoError(t, err)
	second := doc.Installs[0]
	second.Scope = common.InstallationScope{Kind: "tag", Name: "team"}
	doc.Installs = append(doc.Installs, second)
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	drop, err := svc.PlanMachineDrop(doc.Installs[0])
	require.NoError(t, err)
	require.NoError(t, svc.ApplyMachineDrop(context.Background(), *drop))
	_, err = svc.PlanMachineUnlink("project", "codex", link)
	require.ErrorContains(t, err, "covered")
	drop, err = svc.PlanMachineDrop(second)
	require.NoError(t, err)
	require.NoError(t, svc.ApplyMachineDrop(context.Background(), *drop))
	plan, err := svc.PlanMachineUnlink("project", "codex", link)
	require.NoError(t, err)
	doc, err = config.LoadInstallations(filepath.Join(root, "installations.yaml"))
	require.NoError(t, err)
	doc.Installs = append(doc.Installs, second)
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	require.ErrorContains(t, svc.ApplyMachineUnlink(context.Background(), *plan), "covered")
	_, err = os.Readlink(link)
	require.NoError(t, err)
	drop, err = svc.PlanMachineDrop(second)
	require.NoError(t, err)
	require.NoError(t, svc.ApplyMachineDrop(context.Background(), *drop))
	plan, err = svc.PlanMachineUnlink("project", "codex", link)
	require.NoError(t, err)
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.WriteFile(link, []byte("external"), 0o644))
	require.Error(t, svc.ApplyMachineUnlink(context.Background(), *plan))
	data, err := os.ReadFile(link)
	require.NoError(t, err)
	require.Equal(t, "external", string(data))
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink(skill, link))
	plan, err = svc.PlanMachineUnlink("project", "codex", link)
	require.NoError(t, err)
	require.NoError(t, svc.ApplyMachineUnlink(context.Background(), *plan))
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err))
	require.FileExists(t, filepath.Join(skill, "SKILL.md"))
}

func TestMachineUnlinkCoverageHandlesUnknownAndAliasedTargets(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: review\n---\n"), 0o644))
	userDir := filepath.Join(t.TempDir(), "skills")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	link := filepath.Join(userDir, "review")
	require.NoError(t, os.Symlink(skill, link))
	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{skillTarget("client", userDir)}}, common.NewLogger(false))
	require.NoError(t, err)
	doc := config.InstallationsDocument{Version: 1, Installs: []common.Installation{{Skill: "skills/local/unrelated", Target: "missing-client", Scope: common.InstallationScope{Kind: "user"}}}}
	manifest := filepath.Join(root, "installations.yaml")
	require.NoError(t, config.WriteYAMLAtomic(manifest, doc, 0o644))
	_, err = svc.PlanMachineUnlink("", "client", link)
	require.NoError(t, err)
	doc.Installs = append(doc.Installs, common.Installation{Skill: "skills/missing/review", Target: "missing-client", Scope: common.InstallationScope{Kind: "user"}})
	require.NoError(t, config.WriteYAMLAtomic(manifest, doc, 0o644))
	_, err = svc.PlanMachineUnlink("", "client", link)
	require.ErrorContains(t, err, "cannot establish declaration coverage")
	doc.Installs = nil
	require.NoError(t, config.WriteYAMLAtomic(manifest, doc, 0o644))
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".agents"), 0o755))
	require.NoError(t, os.Symlink(userDir, filepath.Join(project, ".agents", "skills")))
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, nil))
	doc, err = config.LoadInstallations(manifest)
	require.NoError(t, err)
	doc.Installs = append(doc.Installs, common.Installation{Skill: "skills/local/review", Target: "codex", Scope: common.InstallationScope{Kind: "directory", Name: "project"}})
	require.NoError(t, config.WriteYAMLAtomic(manifest, doc, 0o644))
	_, err = svc.PlanMachineUnlink("", "client", link)
	require.ErrorContains(t, err, "covered")
}

func TestMachineUnlinkBlocksUnknownDirectoryCoverage(t *testing.T) {
	f := newMachineFixture(t)
	manifest := filepath.Join(f.root, "installations.yaml")
	doc, err := config.LoadInstallations(manifest)
	require.NoError(t, err)
	doc.Installs = []common.Installation{{Skill: "skills/local/review", Target: "claude", Scope: common.InstallationScope{Kind: "directory", Name: "linked"}}}
	require.NoError(t, config.WriteYAMLAtomic(manifest, doc, 0o644))
	pathsFile, err := config.DirectoryPathsFile(f.svc.Cfg.ConfigDir, f.root)
	require.NoError(t, err)
	paths, err := config.LoadDirectoryPaths(pathsFile, doc)
	require.NoError(t, err)
	delete(paths.Paths, "linked")
	require.NoError(t, config.WriteYAMLAtomic(pathsFile, paths, 0o600))
	link := filepath.Join(f.main, ".agents", "skills", "review")
	_, err = f.svc.PlanMachineUnlink("main", "codex", link)
	require.ErrorContains(t, err, "cannot establish declaration coverage")
	_, err = os.Readlink(link)
	require.NoError(t, err)
}
