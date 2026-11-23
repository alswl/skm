package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachineTwentyDirectoriesOverlappingTags(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		dir := filepath.Join(root, "skills", "local", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\n"), 0o644))
	}
	svc := newUpdateSvc(t, root)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("d%02d", i)
		project := filepath.Join(parent, id)
		output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
		require.NoError(t, err, string(output))
		if i != 19 {
			require.NoError(t, os.WriteFile(filepath.Join(project, ".gitignore"), []byte("/.agents/skills/\n/.claude/skills/\n"), 0o644))
		}
		tags := []string{}
		if i < 15 {
			tags = append(tags, "team")
		}
		if i >= 5 {
			tags = append(tags, "extra")
		}
		require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{id: project}, tags))
	}
	external := filepath.Join(parent, "d00", ".agents", "skills", "alpha")
	require.NoError(t, os.MkdirAll(filepath.Dir(external), 0o755))
	require.NoError(t, os.WriteFile(external, []byte("outside"), 0o644))
	plan, err := svc.PlanMachineBatch(MachineBatchRequest{Skills: []string{"alpha", "beta"}, Targets: []string{"codex", "claude"}, Tags: []string{"team", "extra"}})
	require.NoError(t, err)
	require.Len(t, plan.Directories, 20)
	require.Len(t, plan.Items, 80)
	seen := map[string]bool{}
	for _, item := range plan.Items {
		require.False(t, seen[item.Path])
		seen[item.Path] = true
	}
	result, err := svc.ApplyMachineBatch(context.Background(), *plan)
	require.NoError(t, err)
	counts := map[string]int{}
	for _, item := range result.Items {
		counts[item.Status]++
	}
	require.Equal(t, 75, counts["installed"])
	require.Equal(t, 4, counts["unignored"])
	require.Equal(t, 1, counts["conflict"])
	data, err := os.ReadFile(external)
	require.NoError(t, err)
	require.Equal(t, "outside", string(data))
	link := filepath.Join(parent, "d01", ".agents", "skills", "alpha")
	require.NoError(t, os.Remove(link))
	items, err := svc.ScanMachineDirectory("d01")
	require.NoError(t, err)
	foundMissing := false
	for _, item := range items {
		if item.Path == link && item.Status == "missing" && item.Declared {
			foundMissing = true
		}
	}
	require.True(t, foundMissing)
	again, err := svc.PlanMachineRestore(false)
	require.NoError(t, err)
	found := false
	for _, batch := range again.Plans {
		for _, item := range batch.Items {
			if item.Path == link && item.Status == "install" {
				found = true
			}
		}
	}
	require.True(t, found)
}
