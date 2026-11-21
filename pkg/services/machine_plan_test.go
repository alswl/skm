package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/stretchr/testify/require"
)

func TestMachineGitSlotStatus(t *testing.T) {
	root := t.TempDir()
	output, err := exec.Command("git", "init", "-q", root).CombinedOutput()
	require.NoError(t, err, string(output))
	slot := filepath.Join(root, ".agents", "skills", "review")
	status, err := engines.MachineGitSlotStatus(root, slot)
	require.NoError(t, err)
	require.Equal(t, "unignored", status)

	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".agents/skills/\n"), 0o644))
	status, err = engines.MachineGitSlotStatus(root, slot)
	require.NoError(t, err)
	require.Equal(t, "install", status)
}

func TestMachineSafeParentsRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".agents")))
	require.Error(t, machineSafeParents(root, filepath.Join(root, ".agents", "skills")))
	require.Error(t, machineSafeParents(root, filepath.Join(root, "..", "outside")))
	require.NoError(t, machineSafeParents(root, filepath.Join(root, ".claude", "skills")))
}

func TestPreviewMachineTagsDeduplicates(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		skill := filepath.Join(root, "skills", "local", name)
		require.NoError(t, os.MkdirAll(skill, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\n"), 0o644))
	}
	svc := newUpdateSvc(t, root)
	for _, id := range []string{"a", "b"} {
		project := filepath.Join(t.TempDir(), id)
		output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
		require.NoError(t, err, string(output))
		require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{id: project}, []string{"team"}))
	}
	require.NoError(t, svc.TagDirectories(context.Background(), []string{"a"}, []string{"extra"}, nil))
	items, err := svc.PreviewMachineTags([]string{"alpha", "beta"}, []string{"codex", "claude"}, []string{"team", "extra"})
	require.NoError(t, err)
	require.Len(t, items, 8)
	seen := map[string]bool{}
	for _, item := range items {
		require.False(t, seen[item.Path])
		seen[item.Path] = true
		require.Equal(t, "unignored", item.Status)
	}
}

func TestPreviewMachineTagsRejectsTrackedLinkAndInvalidTarget(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: test\n---\n"), 0o644))
	svc := newUpdateSvc(t, root)
	_, err := svc.PreviewMachineTags([]string{"review"}, []string{"unknown"}, []string{"none"})
	require.Error(t, err)

	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	slot := filepath.Join(project, ".agents", "skills", "review")
	require.NoError(t, os.MkdirAll(filepath.Dir(slot), 0o755))
	require.NoError(t, os.Symlink(skill, slot))
	output, err = exec.Command("git", "-C", project, "add", ".agents/skills/review").CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, []string{"team"}))
	items, err := svc.PreviewMachineTags([]string{"review"}, []string{"codex"}, []string{"team"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "conflict", items[0].Status)
}

func TestMachinePlanDeterministicAndReadOnly(t *testing.T) {
	f := newMachineFixture(t)
	manifest := filepath.Join(f.root, "machine.yaml")
	before, err := os.ReadFile(manifest)
	require.NoError(t, err)
	first, err := f.svc.PlanMachineBatch(MachineBatchRequest{Skills: []string{"review", "review"}, Targets: []string{"codex", "claude"}, Tags: []string{"team", "unused"}})
	require.NoError(t, err)
	second, err := f.svc.PlanMachineBatch(MachineBatchRequest{Skills: []string{"review"}, Targets: []string{"claude", "codex", "codex"}, Tags: []string{"unused", "team"}})
	require.NoError(t, err)
	require.Equal(t, first.Digest, second.Digest)
	after, err := os.ReadFile(manifest)
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, err = os.Readlink(filepath.Join(f.main, ".agents", "skills", "review"))
	require.NoError(t, err)
}
