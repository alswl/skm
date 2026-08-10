package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func TestRegistryAdaptsBuiltinProvidersToPluginHost(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Register(NewLocal()))
	p := reg.Providers()[0]
	host, ok := p.(Plugin)
	require.True(t, ok, "built-ins enter the registry through the common plugin host")
	resp, err := host.Handle(t.Context(), PluginRequest{Version: pluginProtocolVersion, Action: "describe"})
	require.NoError(t, err)
	require.Equal(t, PluginKindProvider, resp.Descriptor.Kind)
	require.Equal(t, "local", resp.Descriptor.ID)
}

func TestBuiltinTargetDriverDiffUsesSameDriverAsInstall(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "skills", "local", "demo")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("managed\n"), 0o644))
	targetPath := filepath.Join(root, "target")
	require.NoError(t, os.MkdirAll(filepath.Join(targetPath, "demo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetPath, "demo", "SKILL.md"), []byte("external\n"), 0o644))
	target := common.InstallTarget{Name: "t", Path: targetPath, Accepts: []common.EntryKind{common.KindSkill}, Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink}}
	entry := &common.Entry{Name: "demo", Kind: common.KindSkill, Path: source}
	inst := NewInstaller([]common.InstallTarget{target}, nil)

	diff, err := inst.Diff(context.Background(), entry, target)
	require.NoError(t, err)
	require.Contains(t, diff, "external")
	require.Contains(t, diff, "managed")

	tx := &dal.FileTransaction{}
	_, err = inst.Install(tx, entry, target, true)
	require.NoError(t, err)
	require.Equal(t, common.InstallInstalled, inst.State(entry, target))
}

func TestExternalTargetPluginUsesCommonHostForDiff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preview.sh")
	script := `#!/bin/sh
IFS= read -r line
case "$line" in
  *'"action":"id"'*) echo '{"id":"preview"}' ;;
  *'"action":"diff"'*) echo '{"diff":"external content -> managed content"}' ;;
  *'"action":"inspect"'*) echo '{"dangling":[{"name":"gone","path":"/target/gone"}]}' ;;
  *'"action":"repair"'*) echo '{"result":true}' ;;
  *) echo '{"error":{"code":"protocol_error","message":"unsupported"}}' ;;
esac
`
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	p, err := NewTargetPlugin(path)
	require.NoError(t, err)
	entry := &common.Entry{Name: "demo", Kind: common.KindSkill, Path: "/repo/skills/demo"}
	target := common.InstallTarget{Name: "preview", Path: "/target", Accepts: []common.EntryKind{common.KindSkill}}

	resp, err := p.Handle(t.Context(), PluginRequest{Version: pluginProtocolVersion, Action: "diff", Entry: entry, Target: &target})
	require.NoError(t, err)
	require.Equal(t, PluginKindTarget, p.Descriptor().Kind)
	require.Contains(t, resp.Diff, "external")
	require.Contains(t, resp.Diff, "managed")

	items, err := p.Inspect(t.Context(), target)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "gone", items[0].Name)
	require.Equal(t, "/target/gone", items[0].Path)
	require.Equal(t, "preview", items[0].TargetName)
	require.NoError(t, p.RepairDangling(t.Context(), items[0], target))
}
