package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestMachineTagExitRechecksMembershipAndPreservesOtherDeclarations(t *testing.T) {
	svc := newUpdateSvc(t, t.TempDir())
	manifest := filepath.Join(svc.Cfg.Root, config.InstallationsFile)
	selected := common.Installation{Skill: "skills/local/review", Target: "codex", Scope: common.InstallationScope{Kind: "tag", Name: "team"}}
	other := selected
	other.Target = "claude"
	existing := selected
	existing.Scope = common.InstallationScope{Kind: "directory", Name: "api"}
	doc := config.InstallationsDocument{Version: 1, Installs: []common.Installation{selected, other, existing},
		Directories: map[string]common.Directory{"api": {Tags: []string{"team"}}, "web": {Tags: []string{"team"}}, "ops": {}}}
	require.NoError(t, config.WriteYAMLAtomic(manifest, doc, 0o644))
	file := filepath.Join(t.TempDir(), "review")
	require.NoError(t, os.Symlink("library-review", file))

	plan, err := svc.PlanMachineTagExit(selected, "api", "")
	require.NoError(t, err)
	require.Equal(t, []string{"web"}, plan.Remaining)
	require.NoError(t, svc.TagDirectories(t.Context(), []string{"ops"}, []string{"team"}, nil))
	require.ErrorContains(t, svc.ApplyMachineTagExit(t.Context(), *plan), "stale")
	stale, err := config.LoadInstallations(manifest)
	require.NoError(t, err)
	require.Equal(t, doc.Installs, stale.Installs)

	plan, err = svc.PlanMachineTagExit(selected, "api", "")
	require.NoError(t, err)
	require.NoError(t, svc.ApplyMachineTagExit(t.Context(), *plan))
	updated, err := config.LoadInstallations(manifest)
	require.NoError(t, err)
	web, ops := selected, selected
	web.Scope = common.InstallationScope{Kind: "directory", Name: "web"}
	ops.Scope = common.InstallationScope{Kind: "directory", Name: "ops"}
	require.ElementsMatch(t, []common.Installation{other, existing, web, ops}, updated.Installs)
	require.Equal(t, stale.Directories, updated.Directories)
	link, err := os.Readlink(file)
	require.NoError(t, err)
	require.Equal(t, "library-review", link)
}

func TestMachineTagMembersPreviewRechecksAffectedDeclarations(t *testing.T) {
	svc := newUpdateSvc(t, t.TempDir())
	manifest := filepath.Join(svc.Cfg.Root, config.InstallationsFile)
	first := common.Installation{Skill: "skills/local/review", Target: "codex", Scope: common.InstallationScope{Kind: "tag", Name: "team"}}
	doc := config.InstallationsDocument{Version: 1, Installs: []common.Installation{first},
		Directories: map[string]common.Directory{"api": {Tags: []string{"team", "other"}}, "web": {}}}
	require.NoError(t, config.WriteYAMLAtomic(manifest, doc, 0o644))

	plan, err := svc.PlanMachineTagMembers("team", []string{"web"})
	require.NoError(t, err)
	require.Equal(t, []string{"api"}, plan.Before)
	require.Equal(t, []common.Installation{first}, plan.Declarations)
	second := first
	second.Target = "claude"
	doc.Installs = append(doc.Installs, second)
	require.NoError(t, config.WriteYAMLAtomic(manifest, doc, 0o644))
	require.ErrorContains(t, svc.ApplyMachineTagMembers(t.Context(), *plan), "stale")

	plan, err = svc.PlanMachineTagMembers("team", []string{"web"})
	require.NoError(t, err)
	require.NoError(t, svc.ApplyMachineTagMembers(t.Context(), *plan))
	updated, err := config.LoadInstallations(manifest)
	require.NoError(t, err)
	require.Equal(t, []string{"other"}, updated.Directories["api"].Tags)
	require.Equal(t, []string{"team"}, updated.Directories["web"].Tags)
	require.Equal(t, doc.Installs, updated.Installs)
}
