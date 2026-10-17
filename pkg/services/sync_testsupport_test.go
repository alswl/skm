package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

// gitSyncSource is a local immutable-source fixture. Both revisions are full
// commit IDs so callers cannot accidentally test the moving-branch path.
type gitSyncSource struct {
	Path, Previous, Head string
}

func newGitSyncSource(t *testing.T) gitSyncSource {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	writeFile(t, root, "SKILL.md", frontmatter("review", "Review"))
	writeFile(t, root, "guide.txt", "base\n")
	run("add", ".")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "base")
	previous := run("rev-parse", "HEAD")
	writeFile(t, root, "guide.txt", "upstream\n")
	run("add", ".")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "upstream")
	return gitSyncSource{Path: root, Previous: previous, Head: run("rev-parse", "HEAD")}
}

// legacySyncFixture constructs the smallest 011 repository state used by
// migration tests. It supports both a non-Git snapshot and a Git origin with
// an immutable revision, and it can model an active or retained fork.
func legacySyncFixture(t *testing.T, forkState string, gitOrigin bool) (*Services, string, string) {
	t.Helper()
	root, target := t.TempDir(), t.TempDir()
	source := t.TempDir()
	writeFile(t, source, "SKILL.md", frontmatter("review", "Review"))
	writeFile(t, source, "guide.txt", "base\n")
	origin := common.Origin{Address: source}
	if gitOrigin {
		git := newGitSyncSource(t)
		source = git.Path
		provider := "git"
		origin = common.Origin{Address: source, ProviderID: &provider, RepositoryAddress: source, Revision: git.Previous}
	}
	rel := "skills/local/review"
	writeFile(t, root, filepath.Join(rel, "SKILL.md"), frontmatter("review", "Review"))
	writeFile(t, root, filepath.Join(rel, "guide.txt"), "base\n")
	legacyEntry := filepath.Join(root, ".skm-sync", "entries", dal.SyncKey(rel))
	baseline := filepath.Join(legacyEntry, "upstream-baseline")
	if gitOrigin {
		checkedOut := t.TempDir()
		out, err := exec.Command("git", "-C", source, "worktree", "add", "--detach", checkedOut, origin.Revision).CombinedOutput()
		require.NoError(t, err, string(out))
		require.NoError(t, dal.CopySyncTree(checkedOut, baseline))
		out, err = exec.Command("git", "-C", source, "worktree", "remove", "--force", checkedOut).CombinedOutput()
		require.NoError(t, err, string(out))
	} else {
		require.NoError(t, dal.CopySyncTree(source, baseline))
	}
	base, err := dal.TreeFingerprint(baseline)
	require.NoError(t, err)
	rec := common.SyncRecord{Schema: 1, EntryPath: rel, Name: "review", Generation: 1, Origin: origin, Active: "upstream", UpstreamVersion: origin.Revision, UpstreamDigest: base}
	if forkState == "active" || forkState == "retained" {
		fork := filepath.Join("forks", dal.SyncKey(rel))
		writeFile(t, root, filepath.Join(fork, "SKILL.md"), frontmatter("review", "Review"))
		writeFile(t, root, filepath.Join(fork, "guide.txt"), "custom\n")
		forkDigest, err := dal.TreeFingerprint(filepath.Join(root, fork))
		require.NoError(t, err)
		rec.ForkPath, rec.ForkDigest = fork, forkDigest
		if forkState == "active" {
			rec.Active = "fork"
			rec.ForkVersion = "sha256:" + forkDigest
			require.NoError(t, dal.CopySyncTree(filepath.Join(root, fork), filepath.Join(legacyEntry, "fork-baseline")))
		}
	} else {
		require.Equal(t, "none", forkState, "fork state must be active, retained, or none")
	}
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(root, ".skm-sync", "format.json"), map[string]int{"schema_version": 1}))
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(legacyEntry, "record.json"), rec))
	svc, err := New(newCfg(root, []common.InstallTarget{{Name: "test", Path: target, Accepts: []common.EntryKind{common.KindSkill}, Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink}}}), common.NewLogger(false))
	require.NoError(t, err)
	return svc, source, target
}

func TestLegacySyncFixtureBuildsForkStatesAndImmutableGitSource(t *testing.T) {
	for _, state := range []string{"none", "retained", "active"} {
		t.Run(state, func(t *testing.T) {
			svc, _, _ := legacySyncFixture(t, state, false)
			rec, _, err := svc.Repo.SyncRecord(&common.Entry{Kind: common.KindSkill, Path: filepath.Join(svc.Cfg.Root, "skills/local/review")})
			require.Error(t, err, "legacy state must remain v2-read-only")
			require.Nil(t, rec)
		})
	}
	_, source, _ := legacySyncFixture(t, "active", true)
	git := exec.Command("git", "-C", source, "rev-parse", "HEAD")
	out, err := git.Output()
	require.NoError(t, err)
	require.Len(t, strings.TrimSpace(string(out)), 40)
}

func syncFixture(t *testing.T) (*Services, string, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, dal.InitSyncStore(root)) // v2 regression fixture during migration
	source := t.TempDir()
	target := t.TempDir()
	writeFile(t, source, "SKILL.md", frontmatter("review", "Review"))
	writeFile(t, source, "guide.txt", "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n")
	svc, err := New(newCfg(root, []common.InstallTarget{{Name: "test", Path: target, Accepts: []common.EntryKind{common.KindSkill}, Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink}}}), common.NewLogger(false))
	require.NoError(t, err)
	_, err = svc.Import(context.Background(), source, ImportOptions{})
	require.NoError(t, err)
	return svc, source, target
}
func readSyncFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}
func currentSyncPath(t *testing.T, s *Services) string {
	t.Helper()
	e, err := s.ResolveEntry("review")
	require.NoError(t, err)
	require.NotNil(t, e)
	return filepath.Join(e.Path, "guide.txt")
}
