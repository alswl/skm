package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

// exportFixture builds a git repo with one installable skill and a target.
func exportFixture(t *testing.T) (*Services, *common.InstallTarget, string) {
	t.Helper()
	root := t.TempDir()
	writeSvcFile(t, root, "skills/local/demo/SKILL.md", "---\nname: demo\ndescription: demo\n---\nbody\n")
	require.NoError(t, exec.Command("git", "-C", root, "init", "-q").Run())
	require.NoError(t, exec.Command("git", "-C", root, "add", "-A").Run())
	require.NoError(t, exec.Command("git", "-C", root, "commit", "-q", "-m", "init").Run())
	require.NoError(t, exec.Command("git", "-C", root, "remote", "add", "origin", "git@example.com:team/skills.git").Run())

	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	target := common.InstallTarget{Name: "t", Path: targetDir, Kind: common.KindSkill}

	cfg := newCfg(root, []common.InstallTarget{target})
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	return svc, &target, root
}

func newCfg(root string, targets []common.InstallTarget) *config.Config {
	return &config.Config{Root: root, ConfigDir: filepath.Join(root, ".cfg"), Targets: targets}
}

func TestExportEmitsScopedDeployCommand(t *testing.T) {
	svc, target, _ := exportFixture(t)
	// Install demo so the installed set is non-empty.
	tx := &dal.FileTransaction{}
	_, err := svc.Installer.Install(tx, svc.FindEntry("demo"), *target, false)
	require.NoError(t, err)
	tx.Commit()

	res, err := svc.Export()
	require.NoError(t, err)
	require.Contains(t, res.Command, "skm deploy")
	require.Contains(t, res.Command, "--repo 'git@example.com:team/skills.git'")
	require.Contains(t, res.Command, "--target 't'")
	require.Contains(t, res.Command, "--only 'demo'")
}

func TestExportEmptyInstalledSetEmitsNoCommand(t *testing.T) {
	svc, _, _ := exportFixture(t)
	res, err := svc.Export()
	require.NoError(t, err)
	require.Empty(t, res.Command, "no command when nothing is installed (SC-008)")
}
