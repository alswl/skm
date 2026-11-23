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

func TestMachineApplyIgnoresUnrelatedDirectoryChanges(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: test\n---\n"), 0o644))
	svc := newUpdateSvc(t, root)
	paths := map[string]string{}
	for _, id := range []string{"a", "b"} {
		project := filepath.Join(t.TempDir(), id)
		output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
		require.NoError(t, err, string(output))
		tags := []string{}
		if id == "a" {
			tags = []string{"team"}
		}
		require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{id: project}, tags))
		paths[id] = project
	}
	plan, err := svc.PlanMachineBatch(MachineBatchRequest{Skills: []string{"review"}, Targets: []string{"codex"}, Tags: []string{"team"}, AllowUnignored: true})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(paths["b"], "unrelated"), []byte("changed"), 0o644))
	require.NoError(t, svc.TagDirectories(context.Background(), []string{"b"}, []string{"other"}, nil))
	manifest := filepath.Join(root, "installations.yaml")
	doc, err := config.LoadInstallations(manifest)
	require.NoError(t, err)
	unrelated := common.Installation{Skill: "skills/local/other", Target: "other-client", Scope: common.InstallationScope{Kind: "user"}}
	doc.Installs = append(doc.Installs, unrelated)
	require.NoError(t, config.WriteYAMLAtomic(manifest, doc, 0o644))
	result, err := svc.ApplyMachineBatch(context.Background(), *plan)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "installed", result.Items[0].Status)
	_, err = os.Readlink(filepath.Join(paths["a"], ".agents", "skills", "review"))
	require.NoError(t, err)
	doc, err = config.LoadInstallations(manifest)
	require.NoError(t, err)
	require.Contains(t, doc.Installs, unrelated)
}
