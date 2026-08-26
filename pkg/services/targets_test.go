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
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, config.SettingsFileName),
		[]byte("targets:\n  - name: t\n    platform: p\n    path: "+targetDir+"\n    accepts: [skill]\n    strategies:\n      skill: skill-symlink\n"), 0o644))

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

func TestTargetListReportsDshNameRuleAndBuiltinDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DSH_HOME", "")
	t.Setenv("DSH_AGENTS_HOME", "")
	defaults := config.DefaultTargets()

	cfg := &config.Config{Root: t.TempDir(), ConfigDir: t.TempDir(), Targets: defaults}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	byName := map[string]TargetInfo{}
	for _, ti := range svc.TargetList().Targets {
		byName[ti.Name] = ti
	}
	require.Equal(t, "kebab-case", byName["dsh"].NameRule)
	require.Equal(t, "kebab-case", byName["agents"].NameRule)
	require.Equal(t, byName["dsh"].Path, byName["dsh"].DefaultPath, "a fresh built-in matches its current default")
	require.False(t, byName["dsh"].PathDiverged)
	require.Equal(t, byName["codex"].Path, byName["codex"].DefaultPath, "existing built-ins also report their default path")
	require.Empty(t, byName["claude-skills"].NameRule, "existing built-ins declare no name rule")

	vres := svc.TargetValidate("dsh")
	require.Len(t, vres.Results, 1)
	require.Equal(t, "kebab-case", vres.Results[0].NameRule)
	require.Equal(t, byName["dsh"].Path, vres.Results[0].DefaultPath)
	require.False(t, vres.Results[0].PathDiverged)
}

func TestTargetListFlagsDivergedBuiltinPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DSH_HOME", "")
	t.Setenv("DSH_AGENTS_HOME", "")
	defaults := config.DefaultTargets()

	// Simulate a persisted dsh target whose path no longer matches the
	// current default (e.g. DSH_HOME changed after the file was written).
	var targets []common.InstallTarget
	for _, d := range defaults {
		if d.Name == "dsh" {
			d.Path = filepath.Join(t.TempDir(), "dsh", "skills")
		}
		targets = append(targets, d)
	}
	cfg := &config.Config{Root: t.TempDir(), ConfigDir: t.TempDir(), Targets: targets}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	byName := map[string]TargetInfo{}
	for _, ti := range svc.TargetList().Targets {
		byName[ti.Name] = ti
	}
	require.True(t, byName["dsh"].PathDiverged, "a built-in path that diverges from the current default is flagged")
	require.NotEqual(t, byName["dsh"].DefaultPath, byName["dsh"].Path)
}
