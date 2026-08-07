package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

// T038: legacy migration coverage beyond targets_test.go — CodeFuse path
// preserved, a mixed v1/v2 file, and per-entry drop-and-report end to end
// through Load. T040's fixtures are inline (per T001: matches the codebase's
// existing convention of inline test fixtures over static testdata files).

func TestLoadMigratesLegacyCodeFusePathV1Entry(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))
	cfgDir := t.TempDir()
	// A legacy CodeFuse target, as an old skmgr targets.json would have
	// recorded it: v1 shape, kind:skill, its own (possibly customized) path.
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, targetsFileName),
		[]byte(`[{"name":"codefuse","path":"/custom/codefuse/skills","builtin":true,"kind":"skill"}]`), 0o644))

	cfg, err := Load(root, cfgDir)
	require.NoError(t, err)
	require.Len(t, cfg.Targets, 5, "the 4 built-ins are always merged in alongside the user's codefuse entry")
	byName := map[string]common.InstallTarget{}
	for _, t := range cfg.Targets {
		byName[t.Name] = t
	}
	cf := byName["codefuse"]
	require.Equal(t, "codefuse", cf.Name)
	require.Equal(t, "/custom/codefuse/skills", cf.Path, "the legacy CodeFuse path is preserved, not overwritten by the built-in default")
	require.ElementsMatch(t, []common.EntryKind{common.KindSkill, common.KindCommand}, cf.Accepts)
	require.Equal(t, common.StrategyCommandAdapter, cf.Strategies[common.KindCommand],
		"CodeFuse commands still install via the adapter strategy, exactly as the hardcoded 001 logic did")
	for _, name := range []string{"claude-skills", "claude-commands", "codex", "pi"} {
		require.Contains(t, byName, name, "built-ins stay visible even once the user has a targets.json entry")
	}
}

func TestLoadHandlesMixedV1AndV2Targets(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))
	cfgDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, targetsFileName), []byte(`[
		{"name": "legacy-target", "path": "/p1", "kind": "command", "builtin": false},
		{"name": "modern-target", "platform": "mytool", "path": "/p2",
		 "accepts": ["skill"], "strategies": {"skill": "skill-symlink"}, "builtin": false}
	]`), 0o644))

	cfg, err := Load(root, cfgDir)
	require.NoError(t, err)
	require.Len(t, cfg.Targets, 6, "the 4 built-ins are always merged in alongside the user's 2 entries")
	require.Empty(t, cfg.InvalidTargets)

	byName := map[string]common.InstallTarget{}
	for _, t := range cfg.Targets {
		byName[t.Name] = t
	}
	require.Equal(t, common.StrategyCommandMarker, byName["legacy-target"].Strategies[common.KindCommand])
	require.Equal(t, common.StrategySkillSymlink, byName["modern-target"].Strategies[common.KindSkill])
}

func TestLoadFallsBackToLegacyConfigDirWhenNewDirHasNoTargetsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	legacyDir := filepath.Join(home, LegacyConfigDirName)
	require.NoError(t, os.MkdirAll(legacyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyDir, targetsFileName),
		[]byte(`[{"name":"from-legacy-dir","path":"/legacy/path","kind":"skill","builtin":false}]`), 0o644))

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))

	// No --config passed: configDir resolves to the new default, which has
	// no targets.json, so the legacy dir must be consulted.
	cfg, err := Load(root, "")
	require.NoError(t, err)
	require.Len(t, cfg.Targets, 5, "the 4 built-ins are always merged in alongside the legacy-dir entry")
	require.Equal(t, "from-legacy-dir", cfg.Targets[len(cfg.Targets)-1].Name)
	require.Equal(t, legacyDir, cfg.ConfigDir, "writes must land back in the same file the user already has")
}

func TestLoadPrefersNewConfigDirOverLegacyWhenBothExist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	legacyDir := filepath.Join(home, LegacyConfigDirName)
	require.NoError(t, os.MkdirAll(legacyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyDir, targetsFileName),
		[]byte(`[{"name":"legacy","path":"/legacy","kind":"skill","builtin":false}]`), 0o644))

	newDir := filepath.Join(home, ConfigDirName)
	require.NoError(t, os.MkdirAll(newDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(newDir, targetsFileName),
		[]byte(`[{"name":"new","path":"/new","kind":"skill","builtin":false}]`), 0o644))

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))
	cfg, err := Load(root, "")
	require.NoError(t, err)
	require.Len(t, cfg.Targets, 5, "the 4 built-ins are always merged in alongside the new-dir entry")
	require.Equal(t, "new", cfg.Targets[len(cfg.Targets)-1].Name, "the new-style config dir wins once it exists")
}

func TestLoadIgnoresLegacyDirWhenConfigIsExplicit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacyDir := filepath.Join(home, LegacyConfigDirName)
	require.NoError(t, os.MkdirAll(legacyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyDir, targetsFileName),
		[]byte(`[{"name":"legacy","path":"/legacy","kind":"skill","builtin":false}]`), 0o644))

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))
	explicitCfgDir := t.TempDir() // empty: no targets.json here

	cfg, err := Load(root, explicitCfgDir)
	require.NoError(t, err)
	require.NotEqual(t, "legacy", func() string {
		if len(cfg.Targets) == 0 {
			return ""
		}
		return cfg.Targets[0].Name
	}(), "an explicit --config must never silently fall back to the legacy dir")
	require.Equal(t, explicitCfgDir, cfg.ConfigDir)
}
