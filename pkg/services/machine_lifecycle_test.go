package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/stretchr/testify/require"
)

func TestMachineDeclarationProtectsLifecycle(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: review\n---\n"), 0o644))
	target := t.TempDir()
	link := filepath.Join(target, "review")
	require.NoError(t, os.Symlink(skill, link))
	svc, err := New(&config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{skillTarget("client", target)}}, common.NewLogger(false))
	require.NoError(t, err)
	doc := config.InstallationsDocument{Version: 1, Installs: []common.Installation{{Skill: "skills/local/review", Target: "client", Scope: common.InstallationScope{Kind: "user"}}}}
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	ctx := context.Background()
	_, err = svc.ArchiveWithUninstall(ctx, "review", engines.LifecycleOptions{})
	require.ErrorContains(t, err, "referenced by installation declarations")
	_, err = svc.Delete(ctx, "review", engines.LifecycleOptions{Force: true})
	require.ErrorContains(t, err, "referenced by installation declarations")
	_, err = svc.Normalize(ctx, "review", "other", engines.LifecycleOptions{})
	require.ErrorContains(t, err, "referenced by installation declarations")
	_, err = svc.Convert(ctx, "review", common.KindCommand, engines.LifecycleOptions{})
	require.ErrorContains(t, err, "referenced by installation declarations")
	require.FileExists(t, filepath.Join(skill, "SKILL.md"))
	_, err = os.Readlink(link)
	require.NoError(t, err)
	doc.Installs = nil
	require.NoError(t, config.WriteYAMLAtomic(filepath.Join(root, "installations.yaml"), doc, 0o644))
	_, err = svc.ArchiveWithUninstall(ctx, "review", engines.LifecycleOptions{})
	require.NoError(t, err)
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err))
	require.FileExists(t, filepath.Join(root, "archived", "local", "review", "SKILL.md"))
}

func TestMachineUnreferencedSkillKeepsLifecycleBehavior(t *testing.T) {
	for _, action := range []string{"archive", "delete", "normalize", "convert"} {
		t.Run(action, func(t *testing.T) {
			root := t.TempDir()
			skill := filepath.Join(root, "skills", "local", "review")
			require.NoError(t, os.MkdirAll(skill, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: test\n---\n"), 0o644))
			svc := newUpdateSvc(t, root)
			ctx := context.Background()
			var err error
			switch action {
			case "archive":
				_, err = svc.Archive(ctx, "review", engines.LifecycleOptions{})
				require.NoError(t, err)
				require.FileExists(t, filepath.Join(root, "archived", "local", "review", "SKILL.md"))
			case "delete":
				_, err = svc.Delete(ctx, "review", engines.LifecycleOptions{Force: true})
				require.NoError(t, err)
				require.NoDirExists(t, skill)
			case "normalize":
				_, err = svc.Normalize(ctx, "review", "other", engines.LifecycleOptions{})
				require.NoError(t, err)
				require.FileExists(t, filepath.Join(root, "skills", "other", "review", "SKILL.md"))
			case "convert":
				_, err = svc.Convert(ctx, "review", common.KindCommand, engines.LifecycleOptions{})
				require.NoError(t, err)
				require.FileExists(t, filepath.Join(root, "commands", "local", "review", "command.md"))
			}
		})
	}
}
