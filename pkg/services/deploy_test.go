package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func newDeploySvc(t *testing.T, targets []common.InstallTarget) *Services {
	t.Helper()
	cfg := newCfg(t.TempDir(), targets)
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	return svc
}

func TestDeployPlainDirDirectUse(t *testing.T) {
	src := t.TempDir()
	writeSvcFile(t, src, "skills/local/a/SKILL.md", "---\nname: a\ndescription: a\n---\nbody\n")
	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	target := common.InstallTarget{Name: "t", Path: targetDir, Kind: common.KindSkill}
	svc := newDeploySvc(t, []common.InstallTarget{target})

	res, err := svc.Deploy(context.Background(), DeployOptions{Repo: src})
	require.NoError(t, err)
	require.Equal(t, "direct", res.Clone)
	require.Contains(t, res.Skills, "a")
	require.Len(t, res.Results, 1)

	// The install happened: entry "a" in the source is now installed in target.
	entry := findEntryIn(t, src, "a")
	require.Equal(t, common.InstallInstalled, svc.Installer.State(entry, target))
}

// findEntryIn scans a repository root and returns the named entry.
func findEntryIn(t *testing.T, root, name string) *common.Entry {
	t.Helper()
	for _, e := range NewRepository(root).Scan() {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("entry %q not found in %s", name, root)
	return nil
}

func TestDeployLocalGitRepoPullsFFOnly(t *testing.T) {
	// A local git repo with an upstream (cloned from a bare origin) is pulled
	// ff-only.
	bare := filepath.Join(t.TempDir(), "bare.git")
	require.NoError(t, exec.Command("git", "init", "--bare", "-q", bare).Run())
	src := filepath.Join(t.TempDir(), "work")
	require.NoError(t, exec.Command("git", "clone", "-q", bare, src).Run())
	writeSvcFile(t, src, "skills/local/a/SKILL.md", "---\nname: a\ndescription: a\n---\nbody\n")
	require.NoError(t, exec.Command("git", "-C", src, "add", "-A").Run())
	require.NoError(t, exec.Command("git", "-C", src, "commit", "-q", "-m", "init").Run())
	require.NoError(t, exec.Command("git", "-C", src, "push", "-q", "origin", "HEAD").Run())

	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	svc := newDeploySvc(t, []common.InstallTarget{{Name: "t", Path: targetDir, Kind: common.KindSkill}})

	res, err := svc.Deploy(context.Background(), DeployOptions{Repo: src})
	require.NoError(t, err)
	require.Equal(t, "pulled", res.Clone)
	require.Contains(t, res.Skills, "a")
}

func TestDeployRefusesNonGitNonEmptyCloneTarget(t *testing.T) {
	src := t.TempDir()
	writeSvcFile(t, src, "skills/local/a/SKILL.md", "---\nname: a\ndescription: a\n---\nbody\n")
	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	svc := newDeploySvc(t, []common.InstallTarget{{Name: "t", Path: targetDir, Kind: common.KindSkill}})

	// Pre-create a non-git, non-empty clone cache dir for the URL.
	cacheBase := t.TempDir()
	writeSvcFile(t, cacheBase, "repo/stale.txt", "not a git repo")

	_, err := svc.Deploy(context.Background(), DeployOptions{Repo: "https://github.com/team/skills.git", CacheDir: cacheBase})
	require.Error(t, err, "non-git non-empty clone target must be refused")
}

func TestDeployBareRepoClones(t *testing.T) {
	// Build a working repo, then clone it as a bare repo.
	work := t.TempDir()
	writeSvcFile(t, work, "skills/local/a/SKILL.md", "---\nname: a\ndescription: a\n---\nbody\n")
	require.NoError(t, exec.Command("git", "-C", work, "init", "-q").Run())
	require.NoError(t, exec.Command("git", "-C", work, "add", "-A").Run())
	require.NoError(t, exec.Command("git", "-C", work, "commit", "-q", "-m", "init").Run())
	bare := filepath.Join(t.TempDir(), "bare.git")
	require.NoError(t, exec.Command("git", "clone", "--bare", "-q", work, bare).Run())

	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	svc := newDeploySvc(t, []common.InstallTarget{{Name: "t", Path: targetDir, Kind: common.KindSkill}})

	res, err := svc.Deploy(context.Background(), DeployOptions{Repo: bare, CacheDir: t.TempDir()})
	require.NoError(t, err)
	require.Contains(t, []string{"cloned", "pulled"}, res.Clone)
	require.Contains(t, res.Skills, "a")
}
