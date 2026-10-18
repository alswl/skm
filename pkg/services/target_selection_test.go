package services

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	targetdefs "github.com/alswl/skm/skm/pkg/targets"
	"github.com/stretchr/testify/require"
)

func TestDeployPlatformSelection(t *testing.T) {
	for _, direct := range []bool{false, true} {
		_, opts, _, _ := deployFixture(t)
		home := t.TempDir()
		svc := newDeploySvc(t, targetdefs.Builtins(targetdefs.Context{Home: home, Getenv: func(string) string { return "" }}))
		opts.Targets = []string{"codex", "claude-skills", "claude"}
		opts.NoRepo = direct
		if direct {
			opts.Into = ""
		}
		r, err := svc.Deploy(context.Background(), opts)
		require.NoError(t, err)
		require.Equal(t, []string{"codex", "claude-skills"}, r.Targets)
		require.Len(t, r.Results, 6)
		require.FileExists(t, filepath.Join(home, ".claude/skills/alpha/SKILL.md"))
		require.NoDirExists(t, filepath.Join(home, ".claude/commands"))
	}
}

func TestInstallAndUninstallPlatformSelection(t *testing.T) {
	home := t.TempDir()
	svc := newDeploySvc(t, targetdefs.Builtins(targetdefs.Context{Home: home, Getenv: func(string) string { return "" }}))
	for _, tc := range []struct{ kind, marker, target, installed string }{
		{"skills", "SKILL.md", "claude-skills", "skills/demo/SKILL.md"},
		{"commands", "command.md", "claude-commands", "commands/demo.md"},
	} {
		writeSvcFile(t, svc.Cfg.Root, filepath.Join(tc.kind, "local/demo", tc.marker), "---\nname: demo\ndescription: test\n---\nbody")
		ref := filepath.Join(tc.kind, "local/demo")
		r, err := svc.Install(context.Background(), ref, InstallOptions{Targets: []string{"claude", tc.target}})
		require.NoError(t, err)
		require.Len(t, r.Results, 1)
		require.Equal(t, tc.target, r.Results[0].Target)
		require.FileExists(t, filepath.Join(home, ".claude", tc.installed))
		_, err = svc.Uninstall(context.Background(), ref, InstallOptions{Targets: []string{"claude"}})
		require.NoError(t, err)
		require.NoFileExists(t, filepath.Join(home, ".claude", tc.installed))
	}
}

func TestPlatformSelectionAmbiguityAndExactNames(t *testing.T) {
	a := skillTarget("team-one", filepath.Join(t.TempDir(), "one"))
	a.Platform = "team"
	a.Accepts = []common.EntryKind{common.KindSkill}
	b := skillTarget("team-two", filepath.Join(t.TempDir(), "two"))
	b.Platform = "team"
	b.Accepts = []common.EntryKind{common.KindSkill}
	svc := newDeploySvc(t, []common.InstallTarget{a, b})
	entry := &common.Entry{Name: "demo", Kind: common.KindSkill}
	_, err := svc.selectTargets(entry, []string{"team"})
	require.ErrorContains(t, err, "ambiguous")
	require.ErrorContains(t, err, "team-one, team-two")
	_, err = svc.deployTargets([]string{"team"})
	require.ErrorContains(t, err, "ambiguous")
	selected, err := svc.selectTargets(entry, []string{"team-one"})
	require.NoError(t, err)
	require.Equal(t, []common.InstallTarget{a}, selected)
	_, err = svc.selectTargets(&common.Entry{Kind: common.KindCommand}, []string{"team"})
	require.ErrorContains(t, err, "does not accept")
	_, err = svc.selectTargets(entry, []string{"missing"})
	require.ErrorContains(t, err, "unknown target")

	a.Name = "team"
	a.Platform = "other"
	svc = newDeploySvc(t, []common.InstallTarget{a, b})
	selected, err = svc.selectTargets(entry, []string{"team"})
	require.NoError(t, err)
	require.Equal(t, []common.InstallTarget{a}, selected)
}
