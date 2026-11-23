package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachineDeclarationTracksForkSourceChanges(t *testing.T) {
	f := newMachineFixture(t)
	link := filepath.Join(f.main, ".agents", "skills", "review")
	capture, err := f.svc.PlanMachineCapture("main", "codex", link)
	require.NoError(t, err)
	require.NoError(t, f.svc.ApplyMachineCapture(context.Background(), *capture))
	fork := filepath.Join(f.root, "skills-fork", "local", "review")
	require.NoError(t, os.MkdirAll(fork, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fork, "SKILL.md"), []byte("---\nname: review\ndescription: fork\n---\n"), 0o644))
	items, err := f.svc.ScanMachineDirectory("main")
	require.NoError(t, err)
	found := false
	for _, item := range items {
		if item.Path == link {
			found = true
			require.Equal(t, "source_changed", item.Status)
			require.Equal(t, "skills/local/review", item.Skill)
			require.True(t, item.Declared)
		}
	}
	require.True(t, found)
	plan, err := f.svc.PlanMachineRestore(false)
	require.NoError(t, err)
	require.Len(t, plan.Plans, 1)
	require.Equal(t, "skills/local/review", plan.Plans[0].Declarations[0].Install.Skill)
	require.Equal(t, "conflict", plan.Plans[0].Items[0].Status)
	require.NoError(t, os.Remove(link))
	plan, err = f.svc.PlanMachineRestore(true)
	require.NoError(t, err)
	result, err := f.svc.ApplyMachineRestore(context.Background(), *plan)
	require.NoError(t, err)
	require.Equal(t, "installed", result.Items[0].Status)
	target, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, fork, target)
	items, err = f.svc.ScanMachineDirectory("main")
	require.NoError(t, err)
	for _, item := range items {
		if item.Path == link {
			require.Equal(t, "installed", item.Status)
			require.True(t, item.Declared)
		}
	}
}
