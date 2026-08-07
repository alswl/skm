package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

// T044: removing a target keeps already-installed assets untouched and
// leaves no way to address them through the removed name — no crash, no
// silent deletion, no orphaned link that becomes unaddressable through a
// *different* live target (002-open-provider-target FR-018).
func TestTargetRemoveLeavesInstalledAssetsCoherent(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills", "local", "demo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "skills", "local", "demo", "SKILL.md"),
		[]byte("---\nname: demo\ndescription: demo\n---\nbody\n"), 0o644))

	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	cfgDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "targets.json"),
		[]byte(`[{"name":"t","platform":"p","path":"`+targetDir+`","accepts":["skill"],"strategies":{"skill":"skill-symlink"},"builtin":false}]`), 0o644))

	cfg := &config.Config{Root: root, ConfigDir: cfgDir, Targets: []common.InstallTarget{{
		Name: "t", Platform: "p", Path: targetDir,
		Accepts: []common.EntryKind{common.KindSkill}, Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink},
	}}}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	entry := svc.FindEntry("demo")
	require.NotNil(t, entry)
	tx := &dal.FileTransaction{}
	_, err = svc.Installer.Install(tx, entry, cfg.Targets[0], false)
	require.NoError(t, err)
	tx.Commit()

	linkPath := filepath.Join(targetDir, "demo")
	require.True(t, dal.IsSymlink(linkPath), "install created the managed symlink")

	require.NoError(t, svc.TargetRemove("t"))

	// The already-installed link is untouched: removal only stops future
	// installs, it never deletes existing managed files (FR-018).
	require.True(t, dal.IsSymlink(linkPath), "the managed symlink must survive target removal")
	require.Equal(t, entry.Path, dal.ResolveLink(linkPath), "the link still resolves correctly")

	// The removed target can no longer be addressed: no crash, a clean
	// not-found instead of a stale/dangling reference.
	_, ok := svc.Installer.TargetByName("t")
	require.False(t, ok, "a removed target is not addressable through the installer anymore")
	require.Empty(t, svc.Cfg.Targets, "the removed target is gone from the loaded config")
}
